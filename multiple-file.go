// main.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	kafkaTopic  = "file_transfer"
	mongoDBHost = "localhost:27017"
	chunkSize   = 1 << 20 // 1 MB
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

type FileTransfer struct {
	FromUserID int       `json:"fromUserID"`
	ToUserID   int       `json:"toUserID"`
	FileName   string    `json:"fileName"`
	FileSize   int64     `json:"fileSize"`
	Chunk      []byte    `json:"chunk"`
	ChunkIndex int       `json:"chunkIndex"`
	TotalChunks int      `json:"totalChunks"`
	Time       time.Time `json:"time"`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go kafkaProducer(ctx)
	go kafkaConsumer(ctx)

	http.HandleFunc("/ws", wsHandler)

	server := &http.Server{
		Addr: ":8081",
	}

	go func() {
		log.Fatal(server.ListenAndServe())
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	<-sigChan
	fmt.Println("Shutting down server...")

	if err := server.Shutdown(context.Background()); err != nil {
		log.Fatalf("Server shutdown failed: %s", err)
	}
}

func kafkaProducer(ctx context.Context) {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{"localhost:9092"},
		Topic:    kafkaTopic,
		Balancer: &kafka.LeastBytes{},
	})

	defer writer.Close()

	// Simulate user 1 sending a file to user 74 at random intervals
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Send file transfer data
			fromUserID := 1
			toUserID := 500
			fileName := "200mb_file.txt"

			content, err := ioutil.ReadFile(fileName)
			if err != nil {
				log.Println("Error reading file:", err)
				continue
			}

			fileSize := int64(len(content))
			totalChunks := (fileSize + chunkSize - 1) / chunkSize

			for i := int64(0); i < totalChunks; i++ {
				start := i * chunkSize
				end := (i + 1) * chunkSize
				if end > fileSize {
					end = fileSize
				}

				chunk := content[start:end]

				fileData := FileTransfer{
					FromUserID:  fromUserID,
					ToUserID:    toUserID,
					FileName:    fileName,
					FileSize:    fileSize,
					Chunk:       chunk,
					ChunkIndex:  int(i),
					TotalChunks: int(totalChunks),
					Time:        time.Now(),
				}

				data, err := json.Marshal(fileData)
				if err != nil {
					fmt.Println("Error marshaling file data:", err)
					continue
				}
				err = writer.WriteMessages(ctx, kafka.Message{
					Value: data,
				})
				if err != nil {
					fmt.Println("Error producing message:", err)
				}
			}

			// Sleep for a random duration between 1 and 10 seconds
			sleepDuration := time.Duration(rand.Intn(10)+1) * time.Second
			time.Sleep(sleepDuration)
		}
	}
}

func kafkaConsumer(ctx context.Context) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{"localhost:9092"},
		Topic:     kafkaTopic,
		Partition: 0,
		MinBytes:  10e3,
		MaxBytes:  10e6,
	})

	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				fmt.Println("Error consuming message:", err)
				continue
			}

			var fileData FileTransfer
			err = json.Unmarshal(msg.Value, &fileData)
			if err != nil {
				fmt.Println("Error unmarshaling file data:", err)
				continue
			}

			// Store fileData in MongoDB
			client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://" + mongoDBHost))
			if err != nil {
				fmt.Println("Error creating MongoDB client:", err)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err = client.Connect(ctx)
			if err != nil {
				fmt.Println("Error connecting to MongoDB:", err)
				continue
			}
			defer client.Disconnect(ctx)

			collection := client.Database("fileTransfer").Collection("files")
			_, err = collection.InsertOne(ctx, fileData)
			if err != nil {
				fmt.Println("Error inserting document into MongoDB:", err)
				continue
			}

			fmt.Println("FromUserID:", fileData.FromUserID)
			fmt.Println("ToUserID:", fileData.ToUserID)
			fmt.Println("File name:", fileData.FileName)
			fmt.Println("Chunk index:", fileData.ChunkIndex)
			fmt.Println("Total chunks:", fileData.TotalChunks)
			fmt.Println("Time:", fileData.Time)
		}
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}
	defer conn.Close()

	for {
		// Read incoming message from client
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message from client:", err)
			return
		}

		// Optionally, you can handle the incoming message here

		// Sleep to prevent busy waiting
		time.Sleep(1 * time.Second)
	}
}
