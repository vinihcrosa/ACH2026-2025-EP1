package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"libs/protocol"
	"libs/utils"
	"net"
)

func main() {
	// Create TCP listener on port 8080
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}
	fmt.Println("🚀 TCP server listening on port 8080...")

	for {
		// Accept a connection
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("❌ Error accepting connection:", err)
			continue
		}

		// Handle each connection in a goroutine
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Println("✅ New client connected:", conn.RemoteAddr())

	reader := bufio.NewReader(conn)
	for {
		// Read the message sent by the client
		line, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Println("❌ Client disconnected:", err)
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Println("❌ Error unmarshaling message:", err)
			continue
		}

		switch msg.Type {
		case "handshake":
			var hs protocol.HandshakeData
			utils.ParseData(msg.Data, &hs)
			fmt.Printf("🤝 Handshake received from %s: ClientID=%s, Version=%s\n", conn.RemoteAddr(), hs.ClientID, hs.Version)
		default:
			fmt.Printf("❓ Unknown message type from %s: %s\n", conn.RemoteAddr(), msg.Type)
		}

	}
}
