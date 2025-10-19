package main

import (
	"encoding/json"
	"fmt"
	"libs/protocol"
	"net"
	"sync"
	"time"
)

// MonitorConn represents a watcher client that receives aggregated updates
// about the monitored agents.
type MonitorConn struct {
	remote string
	conn   net.Conn
	enc    *json.Encoder
	mu     sync.Mutex
}

var (
	monitorMu sync.Mutex
	monitors  = make(map[string]*MonitorConn)
)

// send pushes a protocol message to the monitor guarding the encoder with a
// mutex so concurrent broadcasts stay serialized.
func (m *MonitorConn) send(msg protocol.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enc.Encode(msg)
}

// registerMonitor stores a monitor connection so it can receive broadcasts.
func registerMonitor(remote string, conn net.Conn) *MonitorConn {
	monitorMu.Lock()
	defer monitorMu.Unlock()

	mon := &MonitorConn{
		remote: remote,
		conn:   conn,
		enc:    json.NewEncoder(conn),
	}

	monitors[remote] = mon
	return mon
}

// unregisterMonitor forgets a monitor that went offline.
func unregisterMonitor(remote string) {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	delete(monitors, remote)
}

// snapshotMonitors returns a copy of the current monitor list so broadcasts can
// happen without holding the global mutex while sending over the network.
func snapshotMonitors() []*MonitorConn {
	monitorMu.Lock()
	defer monitorMu.Unlock()

	list := make([]*MonitorConn, 0, len(monitors))
	for _, m := range monitors {
		list = append(list, m)
	}

	return list
}

// sendClientsState dumps the summarized state of all connected agents to a
// single monitor client.
func sendClientsState(mon *MonitorConn) error {
	data := protocol.ClientsStateData{
		Clients:     collectClientSummaries(),
		GeneratedAt: time.Now(),
	}

	msg := protocol.Message{
		Type: "clients_state",
		Data: data,
	}

	return mon.send(msg)
}

// broadcastToMonitors fans a message out to every monitor, logging failures but
// keeping the broadcast going for the remaining recipients.
func broadcastToMonitors(msg protocol.Message) {
	for _, mon := range snapshotMonitors() {
		if err := mon.send(msg); err != nil {
			fmt.Printf("❌ Error sending to monitor %s: %v\n", mon.remote, err)
		}
	}
}

// broadcastClientUpdate publishes a fresh snapshot of a client as soon as new
// metrics arrive.
func broadcastClientUpdate(state *ClientState) {
	if state == nil || state.Handshake == nil || state.Handshake.Role != "client" {
		return
	}

	summary := makeClientSummary(state)
	msg := protocol.Message{
		Type: "client_update",
		Data: protocol.ClientUpdateData{Client: summary},
	}

	broadcastToMonitors(msg)
}

// broadcastClientRemoved tells the monitors that a client disconnected.
func broadcastClientRemoved(clientID string) {
	if clientID == "" {
		return
	}

	msg := protocol.Message{
		Type: "client_removed",
		Data: protocol.ClientRemovedData{ClientID: clientID},
	}

	broadcastToMonitors(msg)
}

// sendIntervalSet forwards a request coming from a monitor so the target
// client adjusts its telemetry interval.
func sendIntervalSet(clientID string, intervalMs int64) error {
	cc, ok := getClientConnByID(clientID)
	if !ok {
		return fmt.Errorf("client %s not connected", clientID)
	}

	msg := protocol.Message{
		Type: "set_interval",
		Data: protocol.IntervalUpdateData{IntervalMs: intervalMs},
	}

	return cc.send(msg)
}
