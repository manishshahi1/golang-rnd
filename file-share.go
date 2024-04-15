package main

import (
    "context"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    // "os"
    "path/filepath"
    "time"

    "github.com/gorilla/websocket"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

// WebSocket configuration
var upgrader = websocket.Upgrader{}

// FileMetadata represents the structure of file metadata
type FileMetadata struct {
    FileName    string `json:"filename"`
    ContentType string `json:"contenttype"`
    Sender      string `json:"sender"`
    Receiver    string `json:"receiver"`
}

// MongoDB configuration
var mongoClient *mongo.Client
var mongoDBName = "filesharing"
var mongoCollectionName = "files"

var clients = make(map[*websocket.Conn]bool)
var broadcast = make(chan FileMetadata)

func main() {
    // Initialize MongoDB client
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
    if err != nil {
        log.Fatalf("Failed to connect to MongoDB: %v", err)
    }
    defer client.Disconnect(ctx)
    mongoClient = client

    // Serve the HTML file
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "send_file.html")
    })

    // HTTP handler for WebSocket upgrade
    http.HandleFunc("/ws", handleWebSocket)

    // Serve uploaded files
    http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

    // Start broadcasting files to clients
    go broadcastFiles()

    // Serve HTTP
    log.Println("Server is running on :8080...")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

// handleWebSocket upgrades HTTP connection to WebSocket and handles communication
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Error upgrading to WebSocket:", err)
        return
    }

    // Add client connection to map
    clients[conn] = true

    defer func() {
        conn.Close()
        delete(clients, conn)
    }()

    for {
        // Read the message type and message data from the WebSocket connection
        messageType, data, err := conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("Error reading message from WebSocket: %v", err)
            }
            break
        }

        // Save image data to MongoDB
        fileName := fmt.Sprintf("%d.jpg", time.Now().UnixNano()) // Generate unique filename
        fileMetadata := FileMetadata{
            FileName:    fileName,
            ContentType: http.DetectContentType(data),
            Sender:      r.URL.Query().Get("sender"),
            Receiver:    r.URL.Query().Get("receiver"),
        }
        if err := saveImageToMongo(data, fileName, fileMetadata.Sender, fileMetadata.Receiver); err != nil {
            log.Println("Error saving image to MongoDB:", err)
        }

        // Broadcast the message to all clients except the sender
        broadcast <- fileMetadata

        // Echo the received message back to the client
        err = conn.WriteMessage(messageType, data)
        if err != nil {
            log.Println("Error writing message to WebSocket:", err)
            break
        }
    }
}
// broadcastFiles broadcasts filenames to all connected clients
func broadcastFiles() {
    for {
        // Wait for file metadata to be received on the broadcast channel
        fileMetadata := <-broadcast

        // Iterate over all connected clients
        for client := range clients {
            // Send the filename to the client
            err := client.WriteJSON(fileMetadata.FileName)
            if err != nil {
                log.Println("Error writing JSON to WebSocket:", err)
                client.Close()
                delete(clients, client)
            }
        }
    }
}

// saveImageToMongo saves image data to MongoDB and writes the image file to disk
func saveImageToMongo(data []byte, fileName, sender, receiver string) error {
    // Write the image data to a file in the "uploads" folder
    filePath := filepath.Join("uploads", fileName)
    err := ioutil.WriteFile(filePath, data, 0644)
    if err != nil {
        return fmt.Errorf("failed to write image data to file: %v", err)
    }

    // Insert the image file path, sender, and receiver names into the MongoDB collection
    collection := mongoClient.Database(mongoDBName).Collection(mongoCollectionName)
    _, err = collection.InsertOne(context.Background(), bson.M{
        "filepath": filePath,
        "sender":   sender,
        "receiver": receiver,
    })
    if err != nil {
        return fmt.Errorf("failed to insert file path into MongoDB: %v", err)
    }

    return nil
}