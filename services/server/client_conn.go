package main

import (
	"encoding/json"
	"libs/protocol"
	"net"
	"sync"
)

// ClientConn wraps a raw TCP connection to a monitored client allowing
// concurrent writes through an internal mutex.
type ClientConn struct {
	remote   string
	conn     net.Conn
	enc      *json.Encoder
	mu       sync.Mutex
	clientID string
}

var (
	clientConnMu   sync.Mutex
	clientConns    = make(map[string]*ClientConn)
	clientConnByID = make(map[string]*ClientConn)
)

// send serializes and forwards a protocol message to the connected client.
func (c *ClientConn) send(msg protocol.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(msg)
}

// registerClientConn stores a new client connection keyed by remote address and
// initial client ID, creating the encoder on top of the provided TCP socket.
func registerClientConn(remote string, conn net.Conn, clientID string) *ClientConn {
	clientConnMu.Lock()
	defer clientConnMu.Unlock()

	cc := &ClientConn{
		remote:   remote,
		conn:     conn,
		enc:      json.NewEncoder(conn),
		clientID: clientID,
	}

	clientConns[remote] = cc
	if clientID != "" {
		clientConnByID[clientID] = cc
	}

	return cc
}

// updateClientConnID keeps the cross references in sync when a client sends a
// handshake with a different identifier.
func updateClientConnID(remote, clientID string) {
	clientConnMu.Lock()
	defer clientConnMu.Unlock()

	cc, ok := clientConns[remote]
	if !ok {
		return
	}

	if cc.clientID != "" {
		delete(clientConnByID, cc.clientID)
	}

	cc.clientID = clientID
	if clientID != "" {
		clientConnByID[clientID] = cc
	}
}

// unregisterClientConn drops the references to a client after the connection
// is closed to avoid leaking goroutines or pointers.
func unregisterClientConn(remote string) {
	clientConnMu.Lock()
	defer clientConnMu.Unlock()

	cc, ok := clientConns[remote]
	if !ok {
		return
	}

	if cc.clientID != "" {
		delete(clientConnByID, cc.clientID)
	}

	delete(clientConns, remote)
}

// getClientConnByID retrieves a live client connection by its identifier.
func getClientConnByID(clientID string) (*ClientConn, bool) {
	clientConnMu.Lock()
	defer clientConnMu.Unlock()

	cc, ok := clientConnByID[clientID]
	return cc, ok
}
