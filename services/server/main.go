package main

import (
	"flag"
	"fmt"
	"net"
	"time"
)

// defaultStatsInterval defines how frequently clients should emit usage metrics
// when no specific interval has been negotiated yet.
const defaultStatsInterval = 5 * time.Second

func main() {
	// Parse command-line flags before starting the server.
	port := flag.Int("port", 8080, "TCP port to listen on")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)

	// Start listening for TCP connections on the requested port.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("🚀 TCP server listening on %s...\n", addr)

	// Accept connections indefinitely, delegating the handling to a goroutine
	// so multiple clients can talk to the server at once.
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("❌ Error accepting connection:", err)
			continue
		}
		go handleConnection(conn)
	}
}
