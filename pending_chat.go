package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	WebSocketURL     = "localhost:8080/ws"
	PendingMsgURL    = "localhost:8080/pending"
	MongoURL         = "mongodb://localhost:27017"
	MongoDBName      = "chatDB"
	CollectionName   = "chats"
	StaticFilesPath  = "./static"
	StaticFilesRoute = "/static/"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	clientsMu sync.Mutex
	clients   = make(map[string]*websocket.Conn)

	pendingMu sync.Mutex
	pending   = make(map[string][]Message)

	ctx, cancel = context.WithCancel(context.Background())
	client     *mongo.Client
)

type Message struct {
	ClientID  string    `json:"client_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	var err error
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(MongoURL))
	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/pending", handlePendingMessages)
	http.Handle(StaticFilesRoute, http.StripPrefix(StaticFilesRoute, http.FileServer(http.Dir(StaticFilesPath))))

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	clientID := uuid.New().String()
	log.Printf("Client connected: %s", clientID)

	clientsMu.Lock()
	clients[clientID] = conn
	clientsMu.Unlock()

	sendPendingMessages(clientID)

	for {
		var message Message
		err := conn.ReadJSON(&message)
		if err != nil {
			log.Printf("Error reading from WebSocket: %v", err)
			break
		}

		// Store message in MongoDB
		go storeMessage(message)
	}

	clientsMu.Lock()
	delete(clients, clientID)
	clientsMu.Unlock()

	log.Printf("Client disconnected: %s", clientID)
}

func sendPendingMessages(clientID string) {
	pendingMu.Lock()
	defer pendingMu.Unlock()

	if msgs, ok := pending[clientID]; ok && len(msgs) > 0 {
		conn := clients[clientID]
		for _, msg := range msgs {
			err := conn.WriteJSON(msg)
			if err != nil {
				log.Printf("Error writing to WebSocket: %v", err)
				continue
			}
		}
		delete(pending, clientID)
	}
}

func handlePendingMessages(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")

	pendingMu.Lock()
	defer pendingMu.Unlock()

	if msgs, ok := pending[clientID]; ok && len(msgs) > 0 {
		json.NewEncoder(w).Encode(msgs)
		delete(pending, clientID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func storeMessage(msg Message) {
	collection := client.Database(MongoDBName).Collection(CollectionName)
	_, err := collection.InsertOne(ctx, msg)
	if err != nil {
		log.Printf("Error storing message in MongoDB: %v", err)
	}
}
