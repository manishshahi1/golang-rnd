package main

import (
    "log"
    "net/http"

    "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

func handleUpload(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println(err)
        return
    }
    defer conn.Close()

    for {
        _, data, err := conn.ReadMessage()
        if err != nil {
            log.Println(err)
            break
        }

        // Process the received packet (data)
        log.Printf("Received packet of size %d bytes", len(data))
    }
}

func main() {
    http.HandleFunc("/upload", handleUpload)

    log.Println("Server listening on port 8080...")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatal(err)
    }
}
