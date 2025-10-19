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

// handleConnection wires the handshake, message dispatching and resource cleanup
// for both client and monitor connections.
func handleConnection(conn net.Conn) {
	remote := conn.RemoteAddr().String()
	fmt.Println("✅ New connection:", remote)

	var (
		monitor *MonitorConn
		role    string
	)

	defer func() {
		if monitor != nil {
			unregisterMonitor(remote)
		} else {
			if removed := removeClientState(remote); removed != nil && removed.Handshake != nil && removed.Handshake.Role == "client" {
				broadcastClientRemoved(removed.Handshake.ClientID)
			}
			unregisterClientConn(remote)
		}
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Println("❌ Connection closed:", err)
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Println("❌ Error unmarshaling message:", err)
			continue
		}

		if msg.Type != "handshake" {
			if role == "" {
				fmt.Printf("⚠️  Ignoring %s from %s: handshake not completed\n", msg.Type, remote)
				continue
			}
			if !isMessageAllowedForRole(msg.Type, role) {
				fmt.Printf("🚫 Ignoring %s from %s: role %s not allowed\n", msg.Type, remote, role)
				continue
			}
			if role == "client" {
				if state, ok := getClientState(remote); !ok || state.Handshake == nil {
					fmt.Printf("⚠️  Ignoring %s from %s: client state unavailable\n", msg.Type, remote)
					continue
				}
			}
		}

		switch msg.Type {
		case "handshake":
			var hs protocol.HandshakeData
			if err := utils.ParseData(msg.Data, &hs); err != nil {
				fmt.Println("❌ Error parsing handshake:", err)
				continue
			}
			role = hs.Role
			switch hs.Role {
			case "client":
				state := updateClientState(remote, func(state *ClientState) {
					state.Handshake = &hs
					state.Interval = defaultStatsInterval
				})
				setClientIDForRemote(remote, hs.ClientID)
				registerClientConn(remote, conn, hs.ClientID)
				fmt.Printf("🤝 Client handshake from %s: ClientID=%s, Version=%s\n", remote, hs.ClientID, hs.Version)
				broadcastClientUpdate(state)
				debugState(remote, state)
			case "monitor":
				monitor = registerMonitor(remote, conn)
				fmt.Printf("🛰️  Monitor handshake from %s: ID=%s, Version=%s\n", remote, hs.ClientID, hs.Version)
			default:
				fmt.Printf("⚠️  Unknown role %q from %s\n", hs.Role, remote)
			}
		case "cpu_usage":
			var cpu protocol.CpuUsageData
			if err := utils.ParseData(msg.Data, &cpu); err != nil {
				fmt.Println("❌ Error parsing CPU usage:", err)
				continue
			}
			state := updateClientState(remote, func(state *ClientState) {
				state.CPU = &cpu
			})
			fmt.Printf("📈 CPU update from %s: total %.2f%%\n", remote, cpu.Usage)
			broadcastClientUpdate(state)
			debugState(remote, state)
		case "memory_usage":
			var mem protocol.MemoryUsageData
			if err := utils.ParseData(msg.Data, &mem); err != nil {
				fmt.Println("❌ Error parsing memory usage:", err)
				continue
			}
			state := updateClientState(remote, func(state *ClientState) {
				state.Memory = &mem
			})
			fmt.Printf("🧠 Memory update from %s: used %.2f%%\n", remote, mem.UsedPercent)
			broadcastClientUpdate(state)
			debugState(remote, state)
		case "disk_usage":
			var disk protocol.DiskUsageData
			if err := utils.ParseData(msg.Data, &disk); err != nil {
				fmt.Println("❌ Error parsing disk usage:", err)
				continue
			}
			state := updateClientState(remote, func(state *ClientState) {
				state.Disk = &disk
			})
			fmt.Printf("💾 Disk update from %s: used %.2f%%\n", remote, disk.UsedPercent)
			broadcastClientUpdate(state)
			debugState(remote, state)
		case "general_data":
			var general protocol.GeneralData
			if err := utils.ParseData(msg.Data, &general); err != nil {
				fmt.Println("❌ Error parsing general data:", err)
				continue
			}
			state := updateClientState(remote, func(state *ClientState) {
				state.General = &general
			})
			fmt.Printf("🖥️ General data from %s: %s (%d cores @ %.2f MHz)\n", remote, general.ModelName, general.Cores, general.Mhz)
			broadcastClientUpdate(state)
			debugState(remote, state)
		case "process_usage":
			var proc protocol.ProcessUsageData
			if err := utils.ParseData(msg.Data, &proc); err != nil {
				fmt.Println("❌ Error parsing process usage:", err)
				continue
			}
			state := updateClientState(remote, func(state *ClientState) {
				state.Processes = &proc
			})
			fmt.Printf("📊 Process update from %s: %d entries\n", remote, len(proc.Processes))
			broadcastClientUpdate(state)
			debugState(remote, state)
		case "interval_update":
			var upd protocol.IntervalUpdateData
			if err := utils.ParseData(msg.Data, &upd); err != nil {
				fmt.Println("❌ Error parsing interval update:", err)
				continue
			}
			state := updateClientState(remote, func(state *ClientState) {
				state.Interval = time.Duration(upd.IntervalMs) * time.Millisecond
			})
			fmt.Printf("⏱️ Interval update from %s: %dms\n", remote, upd.IntervalMs)
			broadcastClientUpdate(state)
		case "interval_set_request":
			if monitor == nil {
				fmt.Printf("⚠️  interval_set_request from %s ignored: not a monitor\n", remote)
				continue
			}
			var req protocol.IntervalUpdateData
			if err := utils.ParseData(msg.Data, &req); err != nil {
				fmt.Println("❌ Error parsing interval set request:", err)
				continue
			}
			if req.ClientID == "" || req.IntervalMs <= 0 {
				fmt.Printf("⚠️  Invalid interval request from %s: %+v\n", remote, req)
				continue
			}
			if err := sendIntervalSet(req.ClientID, req.IntervalMs); err != nil {
				fmt.Println("❌ Error sending interval to client:", err)
			}
		case "clients_request":
			if monitor == nil {
				fmt.Printf("⚠️  clients_request from %s ignored: not a monitor\n", remote)
				continue
			}
			if err := sendClientsState(monitor); err != nil {
				fmt.Println("❌ Error sending clients state:", err)
			}
		default:
			fmt.Printf("❓ Unknown message type from %s: %s\n", remote, msg.Type)
		}
	}
}

// isMessageAllowedForRole enforces the list of acceptable messages per role.
func isMessageAllowedForRole(msgType, role string) bool {
	switch role {
	case "client":
		switch msgType {
		case "cpu_usage", "memory_usage", "disk_usage", "general_data", "process_usage", "interval_update":
			return true
		}
	case "monitor":
		if msgType == "clients_request" || msgType == "interval_set_request" {
			return true
		}
	}
	return false
}
