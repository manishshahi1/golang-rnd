package main

import (
        "encoding/json"

    "log"
    "net/http"
    "sync"
    "time"

    "github.com/IBM/sarama"
)

// KafkaProducer holds the Kafka producer instance
var KafkaProducer sarama.SyncProducer

// Message represents the structure of the received message
type Message struct {
    To      string `json:"to"`
    Content string `json:"content"`
}

// PendingMessages represents the structure to store pending messages for each user
type PendingMessages struct {
    sync.Mutex
    Messages map[string][]Message
}

var pendingMessages = PendingMessages{
    Messages: make(map[string][]Message),
}

// Handler function for sending messages to Kafka
func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
    // Decode the JSON message from the request body
    var msg Message
    if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
        http.Error(w, "Failed to decode message", http.StatusBadRequest)
        return
    }

    // Send the message to Kafka
    kafkaMsg := &sarama.ProducerMessage{
        Topic: "group_chat", // Replace "group_chat" with the appropriate Kafka topic
        Value: sarama.StringEncoder(msg.Content),
    }

    partition, offset, err := KafkaProducer.SendMessage(kafkaMsg)
    if err != nil {
        http.Error(w, "Failed to send message to Kafka", http.StatusInternalServerError)
        log.Printf("Failed to send message to Kafka: %v\n", err)
        return
    }

    log.Printf("Message sent to Kafka. Partition: %d, Offset: %d\n", partition, offset)

    // Send a success response
    w.WriteHeader(http.StatusOK)
}

// Function to send pending messages upon user login
func sendPendingMessages(username string) {
    pendingMessages.Lock()
    defer pendingMessages.Unlock()

    messages := pendingMessages.Messages[username]
    for _, msg := range messages {
        kafkaMsg := &sarama.ProducerMessage{
            Topic: "group_chat", // Replace "group_chat" with the appropriate Kafka topic
            Value: sarama.StringEncoder(msg.Content),
        }

        _, _, err := KafkaProducer.SendMessage(kafkaMsg)
        if err != nil {
            log.Printf("Failed to send pending message to Kafka for user %s: %v\n", username, err)
            // Handle failure accordingly
        } else {
            log.Printf("Pending message sent to Kafka for user %s\n", username)
        }
    }

    // Clear pending messages for the user after sending
    delete(pendingMessages.Messages, username)
}

func main() {
    // Create a new Kafka producer
    config := sarama.NewConfig()
    config.Producer.RequiredAcks = sarama.WaitForLocal
    config.Producer.Compression = sarama.CompressionSnappy
    config.Producer.Flush.Frequency = 500 * time.Millisecond
    config.Producer.Return.Successes = true // Add this line to enable returning successes

    brokers := []string{"localhost:9092"}
    var err error
    KafkaProducer, err = sarama.NewSyncProducer(brokers, config)
    if err != nil {
        log.Fatalf("Error creating Kafka producer: %v", err)
    }
    defer KafkaProducer.Close()

    // Serve the chatting2.html file
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "chatting2.html")
    })

    // Set up HTTP server and handlers
    http.HandleFunc("/send-message", sendMessageHandler)

    // Start the HTTP server
    log.Println("Server started on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
