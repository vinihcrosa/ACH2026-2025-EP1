package main

import (
	"encoding/json"
	"fmt"
	"libs/protocol"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("✅ Monitor connected to server, sending handshake...")

	if err := sendMonitorHandshake(conn); err != nil {
		fmt.Println("❌ Error sending monitor handshake:", err)
		return
	}

	fmt.Println("🤝 Monitor handshake sent successfully.")
}

func sendMonitorHandshake(conn net.Conn) error {
	msg := protocol.Message{
		Type: "handshake",
		Data: protocol.HandshakeData{
			ClientID: "monitor1",
			Version:  "1.0.0",
			Role:     "monitor",
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}
