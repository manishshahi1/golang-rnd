package main

import (
    "net/http"
    "log"
    "github.com/gorilla/websocket"
    "github.com/segmentio/kafka-go"
    "github.com/google/uuid"
    "sync"
)

var (
    upgrader = websocket.Upgrader{}
    clients  = make(map[*websocket.Conn]string) // Map to store connected clients and their IDs
    mutex    = sync.Mutex{}                     // Mutex to protect concurrent access to clients map
)

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Error upgrading to WebSocket:", err)
        return
    }
    defer conn.Close()

    // Send identification message to client
    clientId := generateClientId()
    sendMessage(conn, map[string]interface{}{"action": "identification", "clientId": clientId})

    // Register client
    mutex.Lock()
    clients[conn] = clientId
    mutex.Unlock()

    for {
        var message map[string]interface{}
        err := conn.ReadJSON(&message)
        if err != nil {
            log.Println("Error reading message from WebSocket:", err)
            break
        }

        switch message["action"] {
        case "identify":
            // Do nothing, client is already identified
        case "sendMessage":
            // Publish message to Kafka
            messageData := message["message"].(string)
            writer := kafka.NewWriter(kafka.WriterConfig{
                Brokers: []string{"localhost:9092"},
                Topic:   "chat-messages",
            })
            defer writer.Close()

            err = writer.WriteMessages(r.Context(), kafka.Message{
                Value: []byte(messageData),
            })
            if err != nil {
                log.Println("Error publishing message to Kafka:", err)
            }
        case "loadPreviousMessages":
            // Send previous messages to client
            // Implement logic to retrieve previous messages from Kafka and send to the client
            // For now, let's just send a dummy message
            sendMessage(conn, map[string]interface{}{"message": "This is a previous message", "status": "received"})
        }
    }

    // Unregister client
    mutex.Lock()
    delete(clients, conn)
    mutex.Unlock()
}

func generateClientId() string {
    // Generate a UUID (Universal Unique Identifier)
    id := uuid.New()
    return id.String()
}

func sendMessage(conn *websocket.Conn, message map[string]interface{}) {
    mutex.Lock()
    defer mutex.Unlock()

    err := conn.WriteJSON(message)
    if err != nil {
        log.Println("Error sending message to client:", err)
    }
}

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "index.html")
    })

    http.HandleFunc("/ws", handleWebSocket)

    log.Fatal(http.ListenAndServe(":8080", nil))
}
