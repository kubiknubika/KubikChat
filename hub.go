package main

import (
	"fmt"
	"html"
	"log"
	mathrand "math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func StartMathGame() {
	loadData()
	for {
		time.Sleep(120 * time.Second)
		a, b := mathrand.Intn(50), mathrand.Intn(50)
		currentMathAnswer = a + b
		getRoom("#general").Broadcast(Message{
			Username: "QuizBot", Text: fmt.Sprintf("🧮 MATH: %d + %d?", a, b), Type: "system", Room: "#general", Timestamp: time.Now().Unix(),
		}, nil)
	}
}

// --- ROOM LOGIC ---

// Получить комнату (для внутренних нужд). Если нет - создает публичную.
func getRoom(name string) *Room {
	roomMutex.Lock()
	defer roomMutex.Unlock()
	if r, ok := rooms[name]; ok { return r }
	newRoom := &Room{Name: name, Clients: make(map[*Client]bool), History: make([]Message, 0)}
	rooms[name] = newRoom
	return newRoom
}

// Умный вход в комнату (с проверкой пароля)
// Возвращает (Success, ErrorMessage)
func TryJoinRoom(c *Client, roomName, password string) (bool, string) {
	if !strings.HasPrefix(roomName, "#") { roomName = "#" + roomName }
	if c.CurrentRoom == roomName { return false, "Already here." }

	roomMutex.Lock()
	targetRoom, exists := rooms[roomName]
	
	// Если комнаты нет - создаем (с паролем, если он задан)
	if !exists {
		targetRoom = &Room{Name: roomName, Password: password, Clients: make(map[*Client]bool), History: make([]Message, 0)}
		rooms[roomName] = targetRoom
	} else {
		// Если комната есть - проверяем пароль (если он установлен у комнаты)
		if targetRoom.Password != "" && targetRoom.Password != password {
			roomMutex.Unlock()
			return false, "🔒 Access Denied: Wrong password."
		}
	}
	roomMutex.Unlock() // Разблокируем карту комнат, дальше работаем с конкретными комнатами

	// 1. Выходим из старой
	oldRoom := getRoom(c.CurrentRoom)
	oldRoom.Mutex.Lock()
	delete(oldRoom.Clients, c)
	oldRoom.Mutex.Unlock()
	oldRoom.Broadcast(Message{Username: "System", Text: c.Username + " moved to " + roomName, Type: "system", Room: oldRoom.Name, Timestamp: time.Now().Unix()}, nil)

	// 2. Входим в новую
	c.CurrentRoom = roomName
	targetRoom.Mutex.Lock()
	targetRoom.Clients[c] = true
	
	// Шлем историю
	for _, m := range targetRoom.History { c.SendChan <- m }
	targetRoom.Mutex.Unlock()

	c.sendSys("Joined " + roomName)
	targetRoom.Broadcast(Message{Username: "System", Text: c.Username + " entered.", Type: "system", Room: targetRoom.Name, Timestamp: time.Now().Unix()}, nil)
	
	return true, ""
}

func (r *Room) Broadcast(msg Message, sender *Client) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	if msg.Type == "msg" || msg.Type == "action" { r.History = append(r.History, msg); if len(r.History)>50 { r.History=r.History[1:] } }
	for client := range r.Clients {
		msgCopy := msg 
		if sender!=nil && strings.Contains(strings.ToLower(msg.Text), "@"+strings.ToLower(client.Username)) { msgCopy.IsMention=true }
		if msg.Type=="typing" && client==sender { continue }
		select { case client.SendChan <- msgCopy: default: }
	}
}

func (r *Room) BroadcastNoHistory(msg Message) {
	r.Mutex.Lock(); defer r.Mutex.Unlock()
	for client := range r.Clients { select { case client.SendChan <- msg: default: } }
}

// --- CONNECTION HANDLERS ---

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	globalMutex.Lock()
	if bannedIPs[ip] { globalMutex.Unlock(); http.Error(w, "Banned", http.StatusForbidden); return }
	globalMutex.Unlock()

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil { log.Println(err); return }

	client := &Client{
		Conn:         ws,
		Username:     fmt.Sprintf("Guest_%d", mathrand.Intn(1000)),
		LastActivity: time.Now(),
		IP:           ip,
		SendChan:     make(chan Message, 256),
		CurrentRoom:  "#general",
	}

	globalMutex.Lock(); allClients[client] = true; globalMutex.Unlock()
	
	room := getRoom("#general")
	room.Mutex.Lock(); room.Clients[client] = true; room.Mutex.Unlock()

	go client.writePump()
	go client.readPump()

	room.Mutex.Lock(); for _, m := range room.History { client.SendChan <- m }; room.Mutex.Unlock()
	room.Broadcast(Message{Username: "System", Text: client.Username + " joined.", Type: "system", Room: "#general", Timestamp: time.Now().Unix()}, nil)
}

func (c *Client) readPump() {
	defer func() {
		globalMutex.Lock(); delete(allClients, c); globalMutex.Unlock()
		room := getRoom(c.CurrentRoom)
		room.Mutex.Lock(); delete(room.Clients, c); room.Mutex.Unlock()
		close(c.SendChan)
		room.Broadcast(Message{Username: "System", Text: c.Username + " left.", Type: "system", Room: room.Name, Timestamp: time.Now().Unix()}, nil)
	}()
	
	c.Conn.SetReadLimit(MsgLimit + 512)
	c.Conn.SetReadDeadline(time.Now().Add(PongWait))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(PongWait)); return nil })

	for {
		var msg Message
		err := c.Conn.ReadJSON(&msg)
		if err != nil { break }

		c.LastActivity = time.Now()
		if msg.Type == "typing" { getRoom(c.CurrentRoom).Broadcast(Message{Username: c.Username, Type: "typing", Room: c.CurrentRoom}, c); continue }

		msg.Text = strings.TrimSpace(msg.Text)
		if msg.Text == "" { continue }

		globalMutex.Lock(); muted := time.Now().Before(c.MutedUntil); globalMutex.Unlock()
		if muted { c.sendSys("Muted."); continue }

		if currentMathAnswer != -999 {
			if val, err := strconv.Atoi(msg.Text); err == nil && val == currentMathAnswer {
				c.Stars++
				userDatabase[c.Username] = c.Stars
				currentMathAnswer = -999
				getRoom(c.CurrentRoom).Broadcast(Message{Username: "System", Text: fmt.Sprintf("🏆 %s wins!", c.Username), Type: "system", Room: c.CurrentRoom, Timestamp: time.Now().Unix()}, nil)
				triggerSave()
				continue
			}
		}

		msg.Text = html.EscapeString(msg.Text)
		if c.IsInfected { msg.Text = glitchText(msg.Text) }

		if strings.HasPrefix(msg.Text, "/") {
			parts := strings.Fields(msg.Text)
			if cmd, ok := commandRegistry[strings.ToLower(parts[0])]; ok {
				if cmd.AdminOnly && !c.IsAdmin { c.sendSys("⛔ Denied.") } else { cmd.Execute(c, parts[1:]) }
				continue
			}
		}

		msg.Username = c.Username; msg.Stars = c.Stars; msg.Room = c.CurrentRoom; msg.Timestamp = time.Now().Unix()
		if msg.Type == "" { msg.Type = "msg" }
		getRoom(c.CurrentRoom).Broadcast(msg, c)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(time.Second * 5)
	defer func() { ticker.Stop(); c.Conn.Close() }()
	for {
		select {
		case msg, ok := <-c.SendChan:
			c.Conn.SetWriteDeadline(time.Now().Add(10*time.Second))
			if !ok { return }
			c.Conn.WriteJSON(msg)
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10*time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil { return }
		}
	}
}

func (c *Client) sendSys(text string) {
	c.SendChan <- Message{Username: "System", Text: text, Type: "system", Room: c.CurrentRoom, Timestamp: time.Now().Unix()}
}