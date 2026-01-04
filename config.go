package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// --- CONSTANTS ---
const (
	AdminPasswordHash = "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"
	DbFile            = "users_db.bin"
	UploadDir         = "./public/uploads"
	MaxUploadSize     = 5 * 1024 * 1024
	MsgLimit          = 500
	RateLimit         = 200 * time.Millisecond
	PongWait          = 60 * time.Second
	PingPeriod        = (PongWait * 9) / 10
	AFKTimeout        = 5 * time.Minute
	VoteKickPercent   = 0.4
)

var EncryptionKey = []byte("myverystrongpasswordo32bitlength")

// --- STRUCTS ---
type Client struct {
	Conn         *websocket.Conn
	Username     string
	IsAdmin      bool
	IsInfected   bool
	LastActivity time.Time
	LastMsg      time.Time
	MutedUntil   time.Time
	Stars        int
	IP           string
	CurrentRoom  string
	SendChan     chan Message
}

type Message struct {
	Username    string `json:"username"`
	Text        string `json:"text"`
	Type        string `json:"type"`
	Room        string `json:"room"`
	Timestamp   int64  `json:"timestamp"`
	IsMention   bool   `json:"is_mention"`
	OnlineCount int    `json:"online_count"`
	Stars       int    `json:"stars"`
}

type Room struct {
	Name     string
	Password string // <--- НОВОЕ ПОЛЕ
	Clients  map[*Client]bool
	History  []Message
	Mutex    sync.Mutex
}

// --- GLOBAL STATE ---
var (
	upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	
	globalMutex = sync.Mutex{}
	allClients  = make(map[*Client]bool)
	bannedIPs   = make(map[string]bool)
	
	roomMutex = sync.Mutex{}
	rooms     = make(map[string]*Room)

	userDatabase = make(map[string]int)
	saveQueue    = make(chan struct{}, 1)
	
	currentMathAnswer = -999
	
	commandRegistry = make(map[string]Command)
	activeVotes     = make(map[string]map[string]bool)
)

type CommandHandler func(client *Client, args []string)
type Command struct {
	Name, Desc, Usage string
	Execute           CommandHandler
	AdminOnly         bool
}