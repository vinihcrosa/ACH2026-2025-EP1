package main

import (
	"fmt"
	"libs/protocol"
	"sync"
	"time"
)

// ClientState stores the last known snapshot for a connected monitoring agent.
type ClientState struct {
	RemoteAddr string
	Handshake  *protocol.HandshakeData
	CPU        *protocol.CpuUsageData
	Memory     *protocol.MemoryUsageData
	Disk       *protocol.DiskUsageData
	General    *protocol.GeneralData
	Processes  *protocol.ProcessUsageData
	LastUpdate time.Time
	Interval   time.Duration
}

var (
	stateMu       sync.Mutex
	clientStates  = make(map[string]*ClientState)
	clientIDIndex = make(map[string]string)
)

// updateClientState applies the provided mutation while holding the state
// mutex, ensuring the caller receives the updated instance.
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

// getClientState returns the cached state for the given remote endpoint.
func getClientState(remote string) (*ClientState, bool) {
	stateMu.Lock()
	defer stateMu.Unlock()

	st, ok := clientStates[remote]
	return st, ok
}

// setClientIDForRemote associates a client ID with the remote address so we
// can find it later when handling interval commands.
func setClientIDForRemote(remote, clientID string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	clientIDIndex[clientID] = remote
}

// removeClientState clears the cached state for a remote client, returning the
// removed entry so callers can inspect it.
func removeClientState(remote string) *ClientState {
	stateMu.Lock()
	defer stateMu.Unlock()

	st, ok := clientStates[remote]
	if !ok {
		return nil
	}

	if st.Handshake != nil {
		delete(clientIDIndex, st.Handshake.ClientID)
	}

	delete(clientStates, remote)
	return st
}

// collectClientSummaries produces an aggregated view of every connected client
// so monitors can display them all at once.
func collectClientSummaries() []protocol.ClientStateSummary {
	stateMu.Lock()
	defer stateMu.Unlock()

	summaries := make([]protocol.ClientStateSummary, 0, len(clientStates))
	for _, st := range clientStates {
		if st.Handshake == nil || st.Handshake.Role != "client" {
			continue
		}
		summaries = append(summaries, makeClientSummaryUnlocked(st))
	}

	return summaries
}

// makeClientSummary produces a defensive copy of a client state.
func makeClientSummary(state *ClientState) protocol.ClientStateSummary {
	stateMu.Lock()
	defer stateMu.Unlock()
	return makeClientSummaryUnlocked(state)
}

// makeClientSummaryUnlocked expects the mutex to be held already and clones the
// embedded payloads.
func makeClientSummaryUnlocked(state *ClientState) protocol.ClientStateSummary {
	if state == nil {
		return protocol.ClientStateSummary{}
	}

	return protocol.ClientStateSummary{
		RemoteAddr:      state.RemoteAddr,
		Handshake:       cloneHandshake(state.Handshake),
		CPU:             cloneCpuUsage(state.CPU),
		Memory:          cloneMemoryUsage(state.Memory),
		Disk:            cloneDiskUsage(state.Disk),
		General:         cloneGeneralData(state.General),
		Processes:       cloneProcessUsage(state.Processes),
		LastUpdate:      state.LastUpdate,
		StatsIntervalMs: state.Interval.Milliseconds(),
	}
}

// cloneHandshake performs a shallow copy of the handshake data.
func cloneHandshake(hs *protocol.HandshakeData) *protocol.HandshakeData {
	if hs == nil {
		return nil
	}
	copy := *hs
	return &copy
}

// cloneCpuUsage duplicates the CPU usage payload to decouple slices.
func cloneCpuUsage(cpu *protocol.CpuUsageData) *protocol.CpuUsageData {
	if cpu == nil {
		return nil
	}
	copy := *cpu
	copy.CoresUsage = append([]float64(nil), cpu.CoresUsage...)
	return &copy
}

// cloneMemoryUsage duplicates the memory usage payload.
func cloneMemoryUsage(mem *protocol.MemoryUsageData) *protocol.MemoryUsageData {
	if mem == nil {
		return nil
	}
	copy := *mem
	return &copy
}

// cloneDiskUsage duplicates the disk usage payload.
func cloneDiskUsage(disk *protocol.DiskUsageData) *protocol.DiskUsageData {
	if disk == nil {
		return nil
	}
	copy := *disk
	return &copy
}

// cloneGeneralData duplicates the general hardware information payload.
func cloneGeneralData(general *protocol.GeneralData) *protocol.GeneralData {
	if general == nil {
		return nil
	}
	copy := *general
	return &copy
}

// cloneProcessUsage duplicates the top processes payload to avoid sharing the
// slice between goroutines.
func cloneProcessUsage(proc *protocol.ProcessUsageData) *protocol.ProcessUsageData {
	if proc == nil {
		return nil
	}
	clone := *proc
	clone.Processes = append([]protocol.ProcessInfo(nil), proc.Processes...)
	return &clone
}

// debugState prints a compact snapshot of the stored state useful while
// developing or troubleshooting the agents.
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
