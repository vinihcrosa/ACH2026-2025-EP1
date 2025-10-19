package main

import (
	"encoding/json"
	"libs/protocol"
	"net"
)

func sendHandshake(conn net.Conn, clientID string) error {
	msg := protocol.Message{
		Type: "handshake",
		Data: protocol.HandshakeData{
			ClientID: clientID,
			Version:  "1.0.0",
			Role:     "client",
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	writeMu.Lock()
	_, err = conn.Write(append(jsonBytes, '\n'))
	writeMu.Unlock()
	return err
}
