package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func initCommands() {
	reg := func(n, d, u string, h CommandHandler, adm bool) { commandRegistry[n] = Command{Name: n, Desc: d, Usage: u, Execute: h, AdminOnly: adm} }

	reg("/help", "List commands", "/help", cmdHelp, false)
	reg("/nick", "Change name", "/nick [Name]", cmdNick, false)
	
	// JOIN ТЕПЕРЬ ПОДДЕРЖИВАЕТ ПАРОЛЬ
	reg("/join", "Join/Create room", "/join [Room] [Pass]", cmdJoin, false)
	
	reg("/w", "Whisper", "/w [Name] [Msg]", cmdWhisper, false)
	reg("/me", "Action", "/me [Text]", cmdMe, false)
	reg("/roll", "Random num", "/roll", cmdRoll, false)
	reg("/who", "Online in room", "/who", cmdWho, false)
	reg("/auth", "Login", "/auth [Pass]", cmdAuth, false)
	reg("/weather", "Weather", "/weather [City]", cmdWeather, false)
	reg("/votekick", "Vote Kick", "/votekick [Name]", cmdVoteKick, false)
	reg("/burn", "Self-destruct", "/burn [Sec] [Msg]", cmdBurn, false)
	reg("/rooms", "List rooms", "/rooms", cmdRooms, false) // <-- НОВАЯ КОМАНДА

	reg("/kick", "Kick user", "/kick [Name]", cmdKick, true)
	reg("/ban", "Ban user", "/ban [Name]", cmdBan, true)
	reg("/mute", "Mute user", "/mute [Name] [Sec]", cmdMute, true)
	reg("/shout", "Alert", "/shout [Msg]", cmdShout, true)
	reg("/infect", "Glitch user", "/infect [Name]", cmdInfect, true)
	reg("/cure", "Cure user", "/cure [Name]", cmdCure, true)
}

// --- IMPLEMENTATION ---

func cmdJoin(c *Client, args []string) {
	if len(args) == 0 { c.sendSys("Usage: /join [Room] [Password?]"); return }
	
	roomName := args[0]
	password := ""
	if len(args) > 1 { password = args[1] }

	success, msg := TryJoinRoom(c, roomName, password)
	if !success {
		c.sendSys(msg)
	}
}

func cmdRooms(c *Client, args []string) {
	var list []string
	roomMutex.Lock()
	defer roomMutex.Unlock()
	
	for name, r := range rooms {
		r.Mutex.Lock()
		count := len(r.Clients)
		isLocked := r.Password != ""
		r.Mutex.Unlock()
		
		if count > 0 {
			lockIcon := ""
			if isLocked { lockIcon = "🔒 " }
			list = append(list, fmt.Sprintf("%s%s (%d)", lockIcon, name, count))
		}
	}
	sort.Strings(list)
	c.sendSys("Active Rooms: " + strings.Join(list, ", "))
}

// ... Остальные команды БЕЗ ИЗМЕНЕНИЙ (копируем из прошлого раза) ...
func cmdHelp(c *Client, args []string) {
	var names []string
	for _, cmd := range commandRegistry {
		if !cmd.AdminOnly || c.IsAdmin { names = append(names, cmd.Name) }
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString("Available Commands:\n")
	for _, name := range names {
		cmd := commandRegistry[name]
		sb.WriteString(fmt.Sprintf("- %s: %s\n", cmd.Usage, cmd.Desc))
	}
	c.sendSys(sb.String())
}
func cmdInfect(c *Client, args []string) { toggleInfection(c, args, true) }
func cmdCure(c *Client, args []string) { toggleInfection(c, args, false) }
func toggleInfection(c *Client, args []string, state bool) {
	if len(args) == 0 { c.sendSys("Usage: /[cmd] [Name]"); return }
	target := args[0]
	globalMutex.Lock(); defer globalMutex.Unlock()
	for cl := range allClients {
		if strings.EqualFold(cl.Username, target) {
			cl.IsInfected = state
			act := "infected"; if !state { act = "cured" }
			c.sendSys(fmt.Sprintf("%s %s.", target, act))
			return
		}
	}
	c.sendSys("User not found.")
}
func cmdWeather(c *Client, args []string) {
	if len(args)==0{c.sendSys("Usage: /weather [City]");return}
	city := strings.Join(args, "+")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://wttr.in/%s?format=3", city), nil)
		req.Header.Set("User-Agent", "curl/7.64.1")
		if resp, err := (&http.Client{}).Do(req); err == nil {
			defer resp.Body.Close()
			if body, _ := ioutil.ReadAll(resp.Body); resp.StatusCode == 200 {
				c.sendSys(strings.TrimSpace(string(body)))
				return
			}
		}
		c.sendSys("Weather error.")
	}()
}
func cmdBurn(c *Client, args []string) {
    if len(args) < 2 { c.sendSys("Usage: /burn [Sec] [Txt]"); return }
    sec, _ := strconv.Atoi(args[0])
    if sec < 1 || sec > 60 { c.sendSys("Time 1-60s."); return }
    msg := Message{Username: c.Username, Text: strings.Join(args[1:], " "), Type: "burn", Stars: sec, Room: c.CurrentRoom, Timestamp: time.Now().Unix()}
    getRoom(c.CurrentRoom).BroadcastNoHistory(msg)
}
func cmdNick(c *Client, args []string) {
	if len(args) == 0 { return }; newN := args[0]
	if err := validateNickname(newN); err != "" { c.sendSys(err); return }
	globalMutex.Lock()
	for cl := range allClients { if strings.EqualFold(cl.Username, newN) { globalMutex.Unlock(); c.sendSys("Taken."); return } }
	globalMutex.Unlock()
	old := c.Username; c.Username = newN
	if s, ok := userDatabase[newN]; ok { c.Stars = s }
	getRoom(c.CurrentRoom).Broadcast(Message{Username: "System", Text: fmt.Sprintf("%s -> %s", old, newN), Type: "system", Timestamp: time.Now().Unix()}, nil)
}
func cmdWho(c *Client, args []string) {
	r := getRoom(c.CurrentRoom); var n []string; r.Mutex.Lock()
	for cl := range r.Clients { n=append(n, cl.Username) }; r.Mutex.Unlock()
	c.sendSys("In room: " + strings.Join(n, ", "))
}
func cmdMe(c *Client, args []string) { if len(args)>0 { getRoom(c.CurrentRoom).Broadcast(Message{Username:c.Username, Text:strings.Join(args," "), Type:"action", Room:c.CurrentRoom, Timestamp:time.Now().Unix()}, c) } }
func cmdWhisper(c *Client, args []string) {
	if len(args)<2 { return }; tgt, txt := args[0], strings.Join(args[1:], " ")
	globalMutex.Lock(); defer globalMutex.Unlock()
	for cl := range allClients {
		if strings.EqualFold(cl.Username, tgt) {
			cl.SendChan <- Message{Username:c.Username, Text:txt, Type:"private", Timestamp:time.Now().Unix()}
			c.SendChan <- Message{Username:"-> "+tgt, Text:txt, Type:"private", Timestamp:time.Now().Unix()}
			return
		}
	}
	c.sendSys("User not found.")
}
func cmdAuth(c *Client, args []string) { if len(args)>0 && checkPassword(args[0]) { c.IsAdmin=true; c.sendSys("Admin granted.") } }
func cmdRoll(c *Client, args []string) { getRoom(c.CurrentRoom).Broadcast(Message{Username: "System", Text: fmt.Sprintf("%s rolled %d", c.Username, rand.Intn(100)+1), Type: "system", Timestamp: time.Now().Unix()}, nil) }
func cmdVoteKick(c *Client, args []string) { /* Vote kick logic here if needed */ }
func cmdKick(c *Client, args []string) { if len(args)>0 { performBanKick(args[0], false, c.Username) } }
func cmdBan(c *Client, args []string) { if len(args)>0 { performBanKick(args[0], true, c.Username) } }
func cmdShout(c *Client, args []string) { if len(args)>0 { m:=Message{Username:"ANNOUNCE",Text:strings.Join(args," "), Type:"shout", Timestamp:time.Now().Unix()}; roomMutex.Lock(); for _,r:=range rooms{r.Broadcast(m,nil)}; roomMutex.Unlock() } }
func cmdMute(c *Client, args []string) { if len(args)>1 { sec,_:=strconv.Atoi(args[1]); performMute(args[0], sec) } }

func performBanKick(target string, ban bool, admin string) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	
	for cl := range allClients {
		if strings.EqualFold(cl.Username, target) {
			if ban { 
				// Мьютекс уже захвачен выше, просто пишем в карту
				bannedIPs[cl.IP] = true 
			}
			cl.SendChan <- Message{Username: "System", Text: "BYE.", Type: "system"}
			cl.Conn.Close()
			return
		}
	}

}
func performMute(target string, sec int) {
	globalMutex.Lock(); defer globalMutex.Unlock()
	for cl := range allClients {
		if strings.EqualFold(cl.Username, target) {
			cl.MutedUntil = time.Now().Add(time.Duration(sec)*time.Second)
			return
		}
	}
}