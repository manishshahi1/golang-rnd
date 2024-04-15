package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/IBM/sarama"
)

const (
	kafkaBrokers = "localhost:9092"
	topic        = "demo-not-consumed-messages"
)

func main() {
	// Initialize Kafka producer
	producer, err := sarama.NewSyncProducer([]string{kafkaBrokers}, nil)
	if err != nil {
		log.Fatalf("Error creating Kafka producer: %v", err)
	}
	defer producer.Close()

	// Create Kafka topic
	admin, err := sarama.NewClusterAdmin([]string{kafkaBrokers}, nil)
	if err != nil {
		log.Fatalf("Error creating Kafka admin: %v", err)
	}
	defer admin.Close()

	err = admin.CreateTopic(topic, &sarama.TopicDetail{
		NumPartitions:     1,
		ReplicationFactor: 1,
	}, false)
	if err != nil {
		log.Fatalf("Error creating Kafka topic: %v", err)
	}
	fmt.Printf("Topic '%s' created successfully\n", topic)

	// Handle Ctrl+C signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Subscribe to Kafka topic
	consumer, err := sarama.NewConsumer([]string{kafkaBrokers}, nil)
	if err != nil {
		log.Fatalf("Error creating Kafka consumer: %v", err)
	}
	defer consumer.Close()

	partitionConsumer, err := consumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
	if err != nil {
		log.Fatalf("Error creating Kafka partition consumer: %v", err)
	}
	defer partitionConsumer.Close()

	// Start consuming messages
	go func() {
		for {
			select {
			case msg := <-partitionConsumer.Messages():
				fmt.Printf("Consuming message: %s\n", string(msg.Value))
				// Simulate delay in processing message
				time.Sleep(5 * time.Second) // Simulate processing time of 5 seconds
				fmt.Println("Message processed successfully")
			case <-signalChan:
				fmt.Println("Received termination signal. Closing consumer...")
				return
			}
		}
	}()

	// Publish messages
	for i := 1; i <= 5; i++ {
		message := &sarama.ProducerMessage{
			Topic: topic,
			Value: sarama.StringEncoder(fmt.Sprintf("Message %d", i)),
		}
		_, _, err := producer.SendMessage(message)
		if err != nil {
			log.Fatalf("Error publishing message to Kafka: %v", err)
		}
		fmt.Printf("Published message: %s\n", message.Value)

		// Simulate delay between message publishing
		time.Sleep(2 * time.Second) // Simulate delay of 2 seconds between messages
	}

	// Wait for Ctrl+C signal to exit
	<-signalChan
}
