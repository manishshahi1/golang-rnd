package main

import (
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/IBM/sarama"
    "github.com/gorilla/websocket"
)

var producer sarama.AsyncProducer

// Map to store WebSocket connections, keyed by client identifier
var clients = make(map[string]*websocket.Conn)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}

func main() {
    // Set up Kafka producer
    var err error
    config := sarama.NewConfig()
    config.Producer.RequiredAcks = sarama.WaitForLocal
    config.Producer.Return.Successes = true
    producer, err = sarama.NewAsyncProducer([]string{"localhost:9092"}, config)
    if err != nil {
        log.Fatalln("Failed to start Kafka producer:", err)
    }

    defer producer.Close()

    // Handle file upload
    http.HandleFunc("/upload", uploadFile)
    // Handle receiving file from Kafka
    http.HandleFunc("/receive", receiveFile)
    // Handle serving image preview
    http.HandleFunc("/preview", servePreviewImage)
    // Handle WebSocket connections
    http.HandleFunc("/ws", handleWebSocket)

    // Serve HTML file
    http.HandleFunc("/", serveHTMLFile)

    fmt.Println("Server started on port 8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
    // Parse form data
    err := r.ParseMultipartForm(10 << 20) // 10 MB max
    if err != nil {
        http.Error(w, "Unable to parse form", http.StatusBadRequest)
        return
    }

    file, handler, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "Unable to get file", http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Save file to local disk
    tempFile, err := ioutil.TempFile("uploads", "upload-*.png")
    if err != nil {
        http.Error(w, "Unable to save file", http.StatusInternalServerError)
        return
    }
    defer tempFile.Close()

    _, err = io.Copy(tempFile, file)
    if err != nil {
        http.Error(w, "Unable to save file", http.StatusInternalServerError)
        return
    }

    // Send file path to Kafka
    producer.Input() <- &sarama.ProducerMessage{
        Topic: "file-transfer",
        Key:   sarama.StringEncoder(handler.Filename),
        Value: sarama.StringEncoder(tempFile.Name()),
    }

    // Redirect to the main page after upload
    http.Redirect(w, r, "/", http.StatusSeeOther)
}

func receiveFile(w http.ResponseWriter, r *http.Request) {
    // Consume message from Kafka
    msg := <-producer.Successes()
    if msg == nil {
        http.Error(w, "No file received", http.StatusInternalServerError)
        return
    }

    // Convert key and value to strings
    filename := string(msg.Key.(sarama.StringEncoder))
    filepath := string(msg.Value.(sarama.StringEncoder))

    // Log the received file name
    log.Printf("Received file: %s", filename)

    // Serve file to browser
    http.ServeFile(w, r, filepath)
}


func servePreviewImage(w http.ResponseWriter, r *http.Request) {
    // Find the latest uploaded image
    latestFile, err := findLatestFile("uploads", "*.png")
    if err != nil {
        http.Error(w, "No image found", http.StatusInternalServerError)
        return
    }

    // Serve the latest uploaded image
    http.ServeFile(w, r, latestFile)
}

func findLatestFile(dir, pattern string) (string, error) {
    var latestFile string
    var latestModTime int64

    files, err := filepath.Glob(filepath.Join(dir, pattern))
    if err != nil {
        return "", err
    }

    for _, file := range files {
        fileInfo, err := os.Stat(file)
        if err != nil {
            continue
        }
        modTime := fileInfo.ModTime().Unix()
        if modTime > latestModTime {
            latestFile = file
            latestModTime = modTime
        }
    }

    return latestFile, nil
}

func serveHTMLFile(w http.ResponseWriter, r *http.Request) {
    html := `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>File Transfer</title>
    </head>
    <body>
        <h1>File Transfer</h1>
        
        <h2>Upload File</h2>
        <form action="/upload" method="post" enctype="multipart/form-data">
            <input type="file" name="file" id="file">
            <button type="submit">Upload</button>
        </form>

        <h2>Download File</h2>
        <form action="/receive" method="get">
            <button type="submit">Download</button>
        </form>

        <h2>Uploaded Image</h2>
        <img src="/preview" alt="Uploaded Image" id="previewImage">
        
        <script>
            // Auto-refresh the uploaded image every 5 seconds
            setInterval(function() {
                var imgElement = document.getElementById('previewImage');
                var timestamp = new Date().getTime(); // Add timestamp to prevent caching
                imgElement.src = "/preview?" + timestamp;
                    imgElement.width = 200; // Set the width here (e.g., 100 pixels)
            }, 5000);

            // WebSocket connection
            var ws = new WebSocket("ws://" + window.location.host + "/ws");
            ws.onmessage = function(event) {
                console.log("Received message:", event.data);
                // Update UI as needed
            };
        </script>
    </body>
    </html>
    `
    fmt.Fprintf(w, html)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("WebSocket upgrade error:", err)
        return
    }
    defer conn.Close()

    // Generate a unique identifier for the client
    clientID := generateClientID()

    // Register the WebSocket connection with the client identifier
    clients[clientID] = conn
    defer delete(clients, clientID)

    // Listen for messages from the client
    for {
        _, _, err := conn.ReadMessage()
        if err != nil {
            log.Println("WebSocket read error:", err)
            return
        }
    }
}

func generateClientID() string {
    return fmt.Sprintf("%d", time.Now().UnixNano())
}
