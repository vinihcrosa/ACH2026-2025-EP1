package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"libs/protocol"
	"libs/utils"
	"net"
	"time"
)

func listenServer(conn net.Conn, onInterval func(time.Duration), onClose func(error)) {
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			onClose(err)
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Println("❌ Error decoding message from server:", err)
			continue
		}

		switch msg.Type {
		case "set_interval":
			var data protocol.IntervalUpdateData
			if err := utils.ParseData(msg.Data, &data); err != nil {
				fmt.Println("❌ Error parsing set_interval data:", err)
				continue
			}
			if data.IntervalMs <= 0 {
				fmt.Println("⚠️ Invalid interval received from server:", data.IntervalMs)
				continue
			}
			onInterval(time.Duration(data.IntervalMs) * time.Millisecond)
		default:
			// ignore other message types for now
		}
	}
}

func sendIntervalUpdate(conn net.Conn, interval time.Duration) error {
	msg := protocol.Message{
		Type: "interval_update",
		Data: protocol.IntervalUpdateData{IntervalMs: interval.Milliseconds()},
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	writeMu.Lock()
	_, err = conn.Write(append(bytes, '\n'))
	writeMu.Unlock()
	return err
}
