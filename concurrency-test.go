package main

import (
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/IBM/sarama"
)

const (
	kafkaBrokers = "localhost:9092"
	topic        = "chat-room-5"
	numUsers     = 500
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

	// Initialize Kafka consumer
	consumer, err := sarama.NewConsumer([]string{kafkaBrokers}, nil)
	if err != nil {
		log.Fatalf("Error creating Kafka consumer: %v", err)
	}
	defer consumer.Close()

	// Create Kafka partition consumer
	partitionConsumer, err := consumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
	if err != nil {
		log.Fatalf("Error creating Kafka partition consumer: %v", err)
	}
	defer partitionConsumer.Close()

	// Wait group for user goroutines
	var wg sync.WaitGroup

	// Start user goroutines
	for i := 1; i <= numUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			rand.Seed(time.Now().UnixNano())

			// User sends and consumes messages indefinitely
			for {
				message := fmt.Sprintf("User %d: Message %d", userID, rand.Intn(1000))
				producer.SendMessage(&sarama.ProducerMessage{
					Topic: topic,
					Value: sarama.StringEncoder(message),
				})
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond) // Random delay up to 1 second

				select {
				case msg := <-partitionConsumer.Messages():
					fmt.Printf("User %d received: %s\n", userID, string(msg.Value))
				default:
					// No message available
				}
			}
		}(i)
	}

	// Start goroutine to print memory usage statistics
	go func() {
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			fmt.Printf("Memory Usage: Alloc = %d MiB, TotalAlloc = %d MiB, Sys = %d MiB, NumGC = %d\n",
				bToMb(m.Alloc), bToMb(m.TotalAlloc), bToMb(m.Sys), m.NumGC)
			time.Sleep(10 * time.Second) // Print every 10 seconds
		}
	}()

	// Wait for user goroutines to finish
	wg.Wait()
}

// bToMb converts bytes to megabytes.
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
