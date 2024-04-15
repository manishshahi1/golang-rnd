package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "sync"

    "github.com/gorilla/websocket"
    "github.com/segmentio/kafka-go"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

type Client struct {
    ID          string
    Connection  *websocket.Conn
    WriterMutex sync.Mutex
}

type Message struct {
    ClientID  string `json:"clientId"`
    Message   string `json:"message"`
    Seen      bool   `json:"seen"`
    Delivered bool   `json:"delivered"`
}

var clients = make(map[string]*Client)
var kafkaWriter *kafka.Writer
var clientOffsets = make(map[string]int64)

func main() {
    // Set up Kafka writer
    kafkaWriter = kafka.NewWriter(kafka.WriterConfig{
        Brokers:  []string{"localhost:9092"},
        Topic:    "chat",
        Balancer: &kafka.LeastBytes{},
    })

    http.HandleFunc("/ws", handleWebSocket)
    http.HandleFunc("/", handleHome)
    go consumeMessages()
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    // Upgrade HTTP connection to WebSocket
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Error upgrading to WebSocket:", err)
        return
    }

    // Get client ID from query parameter
    clientID := r.URL.Query().Get("clientId")
    if clientID == "" {
        log.Println("Client ID is required")
        return
    }

    // Log client connection
    log.Println("Client connected:", clientID)

    // Create new client instance
    client := &Client{
        ID:          clientID,
        Connection:  conn,
        WriterMutex: sync.Mutex{},
    }
    clients[clientID] = client

    // Set up Kafka reader for this client
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:   []string{"localhost:9092"},
        Topic:     "chat",
        Partition: 0,
        MinBytes:  10e3,
        MaxBytes:  10e6,
    })

    // Fetch only delivered messages for the client from Kafka
    offset, ok := clientOffsets[clientID]
    if !ok {
        // No previous offset found, start from the beginning
        err = reader.SetOffset(kafka.FirstOffset)
        if err != nil {
            log.Println("Error setting offset for client:", err)
            return
        }
    } else {
        err = reader.SetOffset(offset)
        if err != nil {
            log.Println("Error setting offset for client:", err)
            return
        }
    }

    // Read pending messages from Kafka and send them to the client
    for {
        msg, err := reader.ReadMessage(context.Background())
        if err != nil {
            if err.Error() == "kafka: request timed out" {
                // No more messages available
                break
            }
            log.Println("Error reading message from Kafka:", err)
            continue
        }

        // Check if the message is for the requesting client
        if string(msg.Key) == clientID {
            // Mark the message as delivered
            deliveredMessage := Message{
                ClientID:  clientID,
                Message:   string(msg.Value),
                Seen:      false,
                Delivered: true,
            }

            // Send the delivered message to the client
            err := conn.WriteJSON(deliveredMessage)
            if err != nil {
                log.Println("Error sending delivered message to client:", err)
                continue
            }
            log.Println("Delivered message sent to client", clientID, ":", deliveredMessage.Message)
        }
    }

    // Listen for messages from the client
    go func() {
        defer conn.Close()
        for {
            _, msgBytes, err := conn.ReadMessage()
            if err != nil {
                log.Println("Error reading message from client:", err)
                return
            }

            message := string(msgBytes)

            // Check if the message is a reconnection request
            if message == "reconnect" {
                // Update client offset
                clientOffsets[clientID] = reader.Offset() + 1

                // Fetch pending messages for the client from Kafka
                err := fetchPendingMessages(clientID, conn)
                if err != nil {
                    log.Println("Error fetching pending messages:", err)
                    return
                }
                continue
            }

            // Publish message to Kafka
            err = kafkaWriter.WriteMessages(context.Background(), kafka.Message{
                Key:   []byte(clientID),
                Value: []byte(message),
            })
            if err != nil {
                log.Println("Error publishing message to Kafka:", err)
                continue
            }

            // Log sent message
            log.Println("Message sent by client", clientID, ":", message)

            // Send message to all connected clients
            for _, c := range clients {
                c.WriterMutex.Lock()
                err := c.Connection.WriteJSON(Message{ClientID: clientID, Message: message, Seen: false, Delivered: false})
                c.WriterMutex.Unlock()
                if err != nil {
                    log.Println("Error sending message to client:", err)
                    continue
                }
            }
        }
    }()
}

func fetchPendingMessages(clientID string, conn *websocket.Conn) error {
    // Set up Kafka reader for fetching pending messages
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:   []string{"localhost:9092"},
        Topic:     "chat",
        Partition: 0,
        MinBytes:  10e3,
        MaxBytes:  10e6,
    })

    // Seek to the offset for this client
    offset, ok := clientOffsets[clientID]
    if !ok {
        return fmt.Errorf("offset not found for client: %s", clientID)
    }

    err := reader.SetOffset(offset)
    if err != nil {
        return err
    }

    // Iterate through messages to find pending messages for the client
    for {
        msg, err := reader.ReadMessage(context.Background())
        if err != nil {
            if err.Error() == "kafka: request timed out" {
                // No more messages available
                break
            }
            return err
        }

        // Check if the message is for the requesting client
        if string(msg.Key) == clientID {
            // Mark the message as seen
            seenMessage := Message{
                ClientID:  clientID,
                Message:   string(msg.Value),
                Seen:      true,
                Delivered: true,
            }

            // Send the seen message to the client
            err := conn.WriteJSON(seenMessage)
            if err != nil {
                return err
            }
            log.Println("Seen message sent to client", clientID, ":", seenMessage.Message)
        }
    }

    return nil
}

func handleHome(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "resume_msg.html")
}

func consumeMessages() {
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:   []string{"localhost:9092"},
        Topic:     "chat",
        Partition: 0,
        MinBytes:  10e3,
        MaxBytes:  10e6,
    })

    for {
        msg, err := reader.ReadMessage(context.Background())
        if err != nil {
            log.Println("Error reading message from Kafka:", err)
            continue
        }

        clientID := string(msg.Key)
        message := string(msg.Value)

        // Log received message
        log.Println("Message received by client", clientID, ":", message)

        // Send message to all connected clients
        for _, client := range clients {
            client.WriterMutex.Lock()
            err := client.Connection.WriteJSON(Message{ClientID: clientID, Message: message, Seen: false, Delivered: true})
            client.WriterMutex.Unlock()
            if err != nil {
                log.Println("Error sending message to client:", err)
                continue
            }
        }
    }
}
