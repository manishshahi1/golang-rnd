package main

import (
    "log"
    "net/http"
    "strings"

    "github.com/IBM/sarama"
    "github.com/gorilla/websocket"
)

var (
    upgrader = websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
        CheckOrigin: func(r *http.Request) bool {
            return true // Allow all origins
        },
    }

    clients   = make(map[*websocket.Conn]bool)
    broadcast = make(chan []byte)
)

func main() {
    http.HandleFunc("/ws", handleWebSocket)
    go produceMessages()

    log.Println("Server started at :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Failed to upgrade to websocket:", err)
        return
    }
    defer conn.Close()

    clients[conn] = true

    for {
        _, msg, err := conn.ReadMessage()
        if err != nil {
            log.Println("Failed to read message from websocket:", err)
            delete(clients, conn)
            break
        }

        broadcast <- msg
    }
}

func produceMessages() {
    producer, err := sarama.NewSyncProducer(strings.Split("localhost:9092", ","), nil)
    if err != nil {
        log.Println("Failed to start producer:", err)
        return
    }
    defer producer.Close()

    for {
        select {
        case msg := <-broadcast:
            if _, _, err := producer.SendMessage(&sarama.ProducerMessage{
                Topic: "test-topic",
                Value: sarama.StringEncoder(msg),
            }); err != nil {
                log.Println("Failed to send message to Kafka:", err)
            }
        }
    }
}
