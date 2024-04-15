package main

import (
        "bytes"

    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    // "encoding/base64"
    "fmt"
    "io"
    "net/http"
    // "path/filepath"

    "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

// SecretKey is the secret key used for encryption and decryption
var SecretKey = []byte("cdfkpzZWfwI9uIn5") // Change this to your secret key

func main() {
    http.HandleFunc("/ws", handleWebSocket)
    // http.HandleFunc("/crypto.html", serveCryptoHTML)
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "crypto.html")
})
    http.ListenAndServe(":8080", nil)
}

// func serveCryptoHTML(w http.ResponseWriter, r *http.Request) {
//     http.ServeFile(w, r, filepath.Join("public", "crypto.html"))
// }

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        fmt.Println("Error upgrading to WebSocket:", err)
        return
    }
    defer conn.Close()

    for {
        messageType, p, err := conn.ReadMessage()
        if err != nil {
            fmt.Println("Error reading message:", err)
            break
        }

        plaintext, err := decrypt(p)
        if err != nil {
            fmt.Println("Error decrypting message:", err)
            break
        }
        fmt.Printf("Received message: %s\n", plaintext)

        // Echo the received message back to the client after encryption
        encryptedMessage, err := encrypt([]byte("Server received: " + plaintext))
        if err != nil {
            fmt.Println("Error encrypting message:", err)
            break
        }
        if err := conn.WriteMessage(messageType, encryptedMessage); err != nil {
            fmt.Println("Error writing message:", err)
            break
        }
    }
}

func encrypt(data []byte) ([]byte, error) {
    block, err := aes.NewCipher(SecretKey)
    if err != nil {
        return nil, err
    }

    // Add padding to the data
    padding := aes.BlockSize - (len(data) % aes.BlockSize)
    paddedData := append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)

    ciphertext := make([]byte, aes.BlockSize+len(paddedData))
    iv := ciphertext[:aes.BlockSize]
    if _, err := io.ReadFull(rand.Reader, iv); err != nil {
        return nil, err
    }

    mode := cipher.NewCBCEncrypter(block, iv)
    mode.CryptBlocks(ciphertext[aes.BlockSize:], paddedData)
    return ciphertext, nil
}


func decrypt(data []byte) (string, error) {
    block, err := aes.NewCipher(SecretKey)
    if err != nil {
        return "", err
    }
    iv := data[:aes.BlockSize]
    data = data[aes.BlockSize:]
    mode := cipher.NewCBCDecrypter(block, iv)
    mode.CryptBlocks(data, data)
    return string(data), nil
}
