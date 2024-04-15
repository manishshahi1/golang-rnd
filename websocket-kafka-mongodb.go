package main

import (
    "context"
    "encoding/json"
    "log"
    "net/http"
    "time"

    "github.com/gorilla/websocket"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

type Message struct {
    Username string `json:"username"`
    Message  string `json:"message"`
}

var clients = make(map[*websocket.Conn]string) // Connected clients

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Error upgrading to WebSocket:", err)
        return
    }
    defer conn.Close()

    // Read username from client
    _, msg, err := conn.ReadMessage()
    if err != nil {
        log.Println("Error reading username:", err)
        return
    }
    username := string(msg)

    // Add client to the list of connected clients
    clients[conn] = username
    log.Println("Client", username, "connected. Total clients:", len(clients))

    for {
        // Read message from client
        _, msg, err := conn.ReadMessage()
        if err != nil {
            log.Println("Error reading message:", err)
            break
        }

        // Unmarshal JSON message
        var message Message
        if err := json.Unmarshal(msg, &message); err != nil {
            log.Println("Error unmarshalling JSON message:", err)
            continue
        }

        // Save message to MongoDB
        if err := saveMessageToMongoDB(message); err != nil {
            log.Println("Error saving message to MongoDB:", err)
        }

        // Broadcast message to all connected clients
        broadcastMessage(message)
    }

    // Remove client from the list of connected clients when connection is closed
    delete(clients, conn)
    log.Println("Client", username, "disconnected. Total clients:", len(clients))
}

func broadcastMessage(message Message) {
    // Iterate over all connected clients and send the message
    for client := range clients {
        if err := client.WriteJSON(message); err != nil {
            log.Println("Error writing message to client:", err)
        }
    }
}

func saveMessageToMongoDB(message Message) error {
    // Connect to MongoDB
    client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
    if err != nil {
        return err
    }
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    err = client.Connect(ctx)
    if err != nil {
        return err
    }
    defer client.Disconnect(ctx)

    // Insert message into MongoDB collection
    collection := client.Database("infraveo").Collection("messages")
    _, err = collection.InsertOne(ctx, message)
    if err != nil {
        return err
    }

    return nil
}

func main() {
    // Serve static files from the "public" directory
    fs := http.FileServer(http.Dir("public"))
    http.Handle("/", fs)

    http.HandleFunc("/ws", handleWebSocket)
    log.Println("WebSocket server started on port 8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
