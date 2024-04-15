package main

import (
    "context"
    "fmt"
    "log"
    "math/rand"
    "os"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "github.com/segmentio/kafka-go"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
    // Connect to MongoDB
    client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
    if err != nil {
        log.Fatal("Error connecting to MongoDB:", err)
    }
    defer client.Disconnect(context.Background())

    // Connect to Kafka
    kafkaReader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:  []string{"localhost:9092"},
        Topic:    "msgChatHolder",
        MaxWait:  1,       // Wait up to 1 second for new data
        MinBytes: 10e3,    // 10KB
        MaxBytes: 10e6,    // 10MB
    })

    // Graceful shutdown handling
    sigchan := make(chan os.Signal, 1)
    signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

    // Seed the random number generator
    rand.Seed(time.Now().UnixNano())

    // Consume messages from Kafka
    for {
        select {
        case <-sigchan:
            fmt.Println("Received termination signal. Closing Kafka reader.")
            kafkaReader.Close()
            return
        default:
            // Generate a random message
            randomMessage := generateRandomMessage()

            // Print the generated message
            fmt.Printf("Generated random message: %s\n", randomMessage)

            // Save message to MongoDB
            collection := client.Database("your_db").Collection("your_collection")
            _, err := collection.InsertOne(context.Background(), map[string]interface{}{
                "message": randomMessage,
            })
            if err != nil {
                log.Println("Error inserting document into MongoDB:", err)
            } else {
                log.Println("Message saved to MongoDB")
            }
        }
    }
}

// generateRandomMessage generates a random message string
func generateRandomMessage() string {
    var sb strings.Builder

    // Generate a random message of length between 10 and 100 characters
    messageLength := rand.Intn(91) + 10

    // Generate random characters
    for i := 0; i < messageLength; i++ {
        // Generate a random ASCII character in the range [32, 126]
        char := byte(rand.Intn(95) + 32)
        sb.WriteByte(char)
    }

    return sb.String()
}
