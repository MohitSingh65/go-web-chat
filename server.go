package main

import (
	"database/sql"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all connections
		},
	}
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
	db        *sql.DB
)

// Message represents a chat message
type Message struct {
	Username string `json:"username"`
	Text     string `json:"text"`
	Time     string `json:"time"`
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./chat.db")
	if err != nil {
		log.Fatal()
	}

	// Create messages table if it doesnt exist
	_, err = db.Exec(`
          CREATE TABLE IF NOT EXISTS messages (
                  id INTEGER PRIMARY KEY AUTOINCREMENT,
                  username TEXT,
                  text TEXT,
                  time TEXT
          );
  `)
	if err != nil {
		log.Fatal(err)
	}
}

func saveMessage(username, text string) {
	_, err := db.Exec("INSERT INTO messages (username, text, time) VALUES (?, ?, ?)", username, text, time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		log.Println("Error saving message:", err)
	}
}

func loadMessages() []Message {
	rows, err := db.Query("SELECT username, text, time FROM messages ORDER BY time ASC")
	if err != nil {
		log.Println("Error loading messages:", err)
		return nil
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		err := rows.Scan(&msg.Username, &msg.Text, &msg.Time)
		if err != nil {
			log.Println("Error scanning message:", err)
			continue
		}
		messages = append(messages, msg)
	}

	return messages
}

func broadcastMessage(msg Message) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for client := range clients {
		err := client.WriteJSON(msg)
		if err != nil {
			log.Println("Error broadcasting message:", err)
			client.Close()
			delete(clients, client)
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}
	defer conn.Close()

	// Add client to the list
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	// Send chat history to the new client
	messages := loadMessages()
	for _, msg := range messages {
		conn.WriteJSON(msg)
	}

	// Handle incoming messages
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Error reading message:", err)
			break
		}

		// Save and broadcast the message
		saveMessage(msg.Username, msg.Text)
		broadcastMessage(msg)
	}

	// Remove client when they disconnect
	clientsMu.Lock()
	delete(clients, conn)
	clientsMu.Unlock()
}

func main() {
	initDB()

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/", fs)

	// WebSocket endpoint
	http.HandleFunc("/ws", handleWebSocket)

	// Start the server
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
