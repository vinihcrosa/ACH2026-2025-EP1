package main

import (
	"encoding/json"
	"libs/protocol"
	"net"
)

func sendHandshake(conn net.Conn) error {
	msg := protocol.Message{
		Type: "handshake",
		Data: protocol.HandshakeData{
			ClientID: "client123",
			Version:  "1.0.0",
			Role:     "client",
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}
