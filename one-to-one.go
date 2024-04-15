package main

import (
    "log"
    "net/http"
    "os"
    "os/signal"
    "time"

    "github.com/IBM/sarama"
    "github.com/gorilla/websocket"
)

var (
    upgrader = websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
    }
    producer sarama.AsyncProducer
    clients  = make(map[*websocket.Conn]struct{})
)

func initKafkaProducer() {
    config := sarama.NewConfig()
    config.Producer.RequiredAcks = sarama.WaitForLocal
    config.Producer.Compression = sarama.CompressionSnappy
    config.Producer.Flush.Frequency = 500 * time.Millisecond

    brokers := []string{"localhost:9092"}
    var err error
    producer, err = sarama.NewAsyncProducer(brokers, config)
    if err != nil {
        log.Fatalf("Error creating Kafka producer: %v", err)
    }

    go func() {
        for err := range producer.Errors() {
            log.Printf("Failed to produce message: %s\n", err.Error())
        }
    }()
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Error upgrading to WebSocket:", err)
        return
    }
    defer conn.Close()

    log.Println("Client connected")

    clients[conn] = struct{}{}
    defer delete(clients, conn)

    for {
        var msg map[string]string
        err := conn.ReadJSON(&msg)
        if err != nil {
            log.Println("Error reading message:", err)
            break
        }

        to := msg["to"]
        content := msg["content"]

        producer.Input() <- &sarama.ProducerMessage{
            Topic: to,
            Value: sarama.StringEncoder(content),
        }

        // Send the received message back to all clients
        for client := range clients {
            err := client.WriteJSON(msg)
            if err != nil {
                log.Println("Error sending message to client:", err)
            }
        }
    }

    log.Println("Client disconnected")
}

func main() {
    initKafkaProducer()
    defer producer.Close()

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "chatting.html")
    })

    http.HandleFunc("/ws", handleWebSocket)

    log.Println("Server started on :8083")

    // Start the HTTP server
    go func() {
        if err := http.ListenAndServe(":8083", nil); err != nil {
            log.Fatalf("Error starting server: %v", err)
        }
    }()

    // Graceful shutdown
    interrupt := make(chan os.Signal, 1)
    signal.Notify(interrupt, os.Interrupt)
    <-interrupt
    log.Println("Shutting down...")
}
