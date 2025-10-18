package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"libs/protocol"
	"libs/utils"
	"net"
	"sync"
	"time"
)

type ClientState struct {
	RemoteAddr string
	Handshake  *protocol.HandshakeData
	CPU        *protocol.CpuUsageData
	Memory     *protocol.MemoryUsageData
	Disk       *protocol.DiskUsageData
	General    *protocol.GeneralData
	Processes  *protocol.ProcessUsageData
	LastUpdate time.Time
}

var (
	stateMu      sync.Mutex
	clientStates = make(map[string]*ClientState)
)

func updateClientState(remote string, update func(state *ClientState)) *ClientState {
	stateMu.Lock()
	defer stateMu.Unlock()

	state, ok := clientStates[remote]
	if !ok {
		state = &ClientState{RemoteAddr: remote}
		clientStates[remote] = state
	}
	update(state)
	state.LastUpdate = time.Now()
	return state
}

func getClientState(remote string) (*ClientState, bool) {
	stateMu.Lock()
	defer stateMu.Unlock()
	st, ok := clientStates[remote]
	return st, ok
}

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

	remote := conn.RemoteAddr().String()
	fmt.Println("✅ New client connected:", remote)

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

		// Only allow messages from authorized roles
		if msg.Type != "handshake" {
			state, ok := getClientState(remote)
			if !ok || state.Handshake == nil {
				fmt.Printf("⚠️  Ignoring %s from %s: handshake not completed\n", msg.Type, remote)
				continue
			}
			if !isMessageAllowedForRole(msg.Type, state.Handshake.Role) {
				fmt.Printf("🚫 Ignoring %s from %s: role %s not allowed\n", msg.Type, remote, state.Handshake.Role)
				continue
			}
		}

		switch msg.Type {
		case "handshake":
			var hs protocol.HandshakeData
			if err := utils.ParseData(msg.Data, &hs); err != nil {
				fmt.Println("❌ Error parsing handshake:", err)
				continue
			}
			state := updateClientState(remote, func(state *ClientState) {
				state.Handshake = &hs
			})
			fmt.Printf("🤝 Handshake from %s: ClientID=%s, Version=%s, Role=%s\n", remote, hs.ClientID, hs.Version, hs.Role)
			debugState(remote, state)
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
			fmt.Printf("🖥️ General data from %s: %s (%d cores @ %.2f MHz)\n",
				remote, general.ModelName, general.Cores, general.Mhz)
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
			debugState(remote, state)
		case "clients_request":
			if err := sendClientsState(conn); err != nil {
				fmt.Println("❌ Error sending clients state:", err)
			}
		default:
			fmt.Printf("❓ Unknown message type from %s: %s\n", remote, msg.Type)
		}

	}
}

func isMessageAllowedForRole(msgType, role string) bool {
	switch role {
	case "client":
		switch msgType {
		case "cpu_usage", "memory_usage", "disk_usage", "general_data", "process_usage":
			return true
		}
	case "monitor":
		if msgType == "clients_request" {
			return true
		}
	}
	return false
}

func sendClientsState(conn net.Conn) error {
	data := protocol.ClientsStateData{
		Clients:     collectClientSummaries(),
		GeneratedAt: time.Now(),
	}

	msg := protocol.Message{
		Type: "clients_state",
		Data: data,
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}

func collectClientSummaries() []protocol.ClientStateSummary {
	stateMu.Lock()
	defer stateMu.Unlock()

	summaries := make([]protocol.ClientStateSummary, 0, len(clientStates))
	for _, st := range clientStates {
		if st.Handshake == nil || st.Handshake.Role != "client" {
			continue
		}
		summaries = append(summaries, protocol.ClientStateSummary{
			RemoteAddr: st.RemoteAddr,
			Handshake:  cloneHandshake(st.Handshake),
			CPU:        cloneCpuUsage(st.CPU),
			Memory:     cloneMemoryUsage(st.Memory),
			Disk:       cloneDiskUsage(st.Disk),
			General:    cloneGeneralData(st.General),
			Processes:  cloneProcessUsage(st.Processes),
			LastUpdate: st.LastUpdate,
		})
	}

	return summaries
}

func cloneHandshake(hs *protocol.HandshakeData) *protocol.HandshakeData {
	if hs == nil {
		return nil
	}
	copy := *hs
	return &copy
}

func cloneCpuUsage(cpu *protocol.CpuUsageData) *protocol.CpuUsageData {
	if cpu == nil {
		return nil
	}
	copy := *cpu
	copy.CoresUsage = append([]float64(nil), cpu.CoresUsage...)
	return &copy
}

func cloneMemoryUsage(mem *protocol.MemoryUsageData) *protocol.MemoryUsageData {
	if mem == nil {
		return nil
	}
	copy := *mem
	return &copy
}

func cloneDiskUsage(disk *protocol.DiskUsageData) *protocol.DiskUsageData {
	if disk == nil {
		return nil
	}
	copy := *disk
	return &copy
}

func cloneGeneralData(general *protocol.GeneralData) *protocol.GeneralData {
	if general == nil {
		return nil
	}
	copy := *general
	return &copy
}

func cloneProcessUsage(proc *protocol.ProcessUsageData) *protocol.ProcessUsageData {
	if proc == nil {
		return nil
	}
	clone := *proc
	clone.Processes = append([]protocol.ProcessInfo(nil), proc.Processes...)
	return &clone
}

func debugState(remote string, state *ClientState) {
	fmt.Printf("🗂️  State snapshot for %s (updated %s)\n", remote, state.LastUpdate.Format(time.RFC3339))
	if state.Handshake != nil {
		fmt.Printf("   - Handshake: ClientID=%s Version=%s Role=%s\n", state.Handshake.ClientID, state.Handshake.Version, state.Handshake.Role)
	}
	if state.General != nil {
		fmt.Printf("   - General: %s, %d cores @ %.2f MHz\n", state.General.ModelName, state.General.Cores, state.General.Mhz)
	}
	if state.CPU != nil {
		fmt.Printf("   - CPU: %.2f%% total (%d cores)\n", state.CPU.Usage, len(state.CPU.CoresUsage))
	}
	if state.Memory != nil {
		fmt.Printf("   - Memory: %.2f%% used (%d/%d)\n", state.Memory.UsedPercent, state.Memory.Used, state.Memory.Total)
	}
	if state.Disk != nil {
		fmt.Printf("   - Disk: %.2f%% used (%d/%d)\n", state.Disk.UsedPercent, state.Disk.Used, state.Disk.Total)
	}
	if state.Processes != nil {
		top := len(state.Processes.Processes)
		if top > 3 {
			top = 3
		}
		for i := 0; i < top; i++ {
			p := state.Processes.Processes[i]
			fmt.Printf("   - Proc #%d: PID=%d Name=%s CPU=%.2f%% Mem=%.2fMB\n", i+1, p.PID, p.Name, p.CPUPercent, p.MemoryMB)
		}
	}
}
