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

	if err := requestClientsState(conn); err != nil {
		fmt.Println("❌ Error requesting clients state:", err)
		return
	}
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

func requestClientsState(conn net.Conn) error {
	req := protocol.Message{
		Type: "clients_request",
		Data: protocol.ClientsRequestData{},
	}

	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if _, err := conn.Write(append(jsonBytes, '\n')); err != nil {
		return err
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}

	var resp protocol.Message
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}

	if resp.Type != "clients_state" {
		return fmt.Errorf("unexpected response type: %s", resp.Type)
	}

	var data protocol.ClientsStateData
	if err := utils.ParseData(resp.Data, &data); err != nil {
		return err
	}

	fmt.Printf("📋 %d client(s) connected (snapshot %s)\n", len(data.Clients), data.GeneratedAt.Format(time.RFC3339))
	for _, client := range data.Clients {
		id := "unknown"
		version := ""
		if client.Handshake != nil {
			id = client.Handshake.ClientID
			version = client.Handshake.Version
		}
		fmt.Printf("- %s | id=%s | version=%s | updated=%s\n",
			client.RemoteAddr,
			id,
			version,
			client.LastUpdate.Format(time.RFC3339),
		)

		if client.CPU != nil {
			fmt.Printf("  CPU: %.2f%% (cores=%d)\n", client.CPU.Usage, len(client.CPU.CoresUsage))
		}
		if client.Memory != nil {
			fmt.Printf("  Memory: %.2f%% used\n", client.Memory.UsedPercent)
		}
		if client.Disk != nil {
			fmt.Printf("  Disk: %.2f%% used\n", client.Disk.UsedPercent)
		}
		if client.General != nil {
			fmt.Printf("  General: %s (%d cores @ %.2f MHz)\n", client.General.ModelName, client.General.Cores, client.General.Mhz)
		}
		if client.Processes != nil {
			fmt.Printf("  Processes tracked: %d\n", len(client.Processes.Processes))
		}
	}

	return nil
}
