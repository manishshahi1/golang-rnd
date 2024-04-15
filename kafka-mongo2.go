package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// User represents a chat user
type User struct {
	ID   string
	Name string
}

func main() {
	// Initialize Kafka producer
	kafkaWriter := initKafkaProducer()

	// Initialize MongoDB client
	client := initMongoDB()

	// Create two users
	manish := User{ID: "manish", Name: "Manish"}
	anandSir := User{ID: "anand_sir", Name: "Anand Sir"}

	// Simulate conversation and save messages to MongoDB
	sendChatMessages(kafkaWriter, client, manish, anandSir)
}

// initKafkaProducer initializes a Kafka producer
func initKafkaProducer() *kafka.Writer {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{"localhost:9092"},
		Topic:    "chat_messages_new",
		Balancer: &kafka.LeastBytes{},
	})
	return writer
}

// initMongoDB initializes a MongoDB client
func initMongoDB() *mongo.Client {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	return client
}

// sendChatMessages sends specific chat messages between two users and saves them to MongoDB
func sendChatMessages(writer *kafka.Writer, client *mongo.Client, user1, user2 User) {
	conversation := []struct {
		Sender   User
		Receiver User
		Message  string
	}{
		{Sender: user1, Receiver: user2, Message: "hi sir"},
		{Sender: user2, Receiver: user1, Message: "hello"},
		{Sender: user1, Receiver: user2, Message: "Sir this is a demo of golang, kafka, and mongodb chat message saving."},
		{Sender: user2, Receiver: user1, Message: "Great!"},
		{Sender: user1, Receiver: user2, Message: "Yay!"},
		{Sender: user2, Receiver: user1, Message: "Well done!"},
		{Sender: user1, Receiver: user2, Message: "Thanks sir"},
	}

	for _, conv := range conversation {
		fmt.Printf("%s: %s\n", conv.Sender.Name, conv.Message)

		// Send message to Kafka
		err := writer.WriteMessages(context.Background(), kafka.Message{
			Key:   []byte(conv.Sender.ID),
			Value: []byte(conv.Message),
		})
		if err != nil {
			log.Println("Error sending message to Kafka:", err)
		}

		// Save message to MongoDB
		collection := client.Database("chat_app").Collection("messages")
		_, err = collection.InsertOne(context.Background(), bson.M{
			"sender":    conv.Sender.Name,
			"receiver":  conv.Receiver.Name,
			"message":   conv.Message,
			"timestamp": time.Now(),
		})
		if err != nil {
			log.Println("Error inserting document into MongoDB:", err)
		}
		time.Sleep(time.Second) // Add a delay between messages
	}
}
