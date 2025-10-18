package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	// Connect to the server
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("✅ Connected to the server. Type messages:")

	// Send handshake message
	err = sendHandshake(conn)
	if err != nil {
		fmt.Println("❌ Error sending handshake:", err)
		return
	}

	defaultInterval := 5 * time.Second
	intervalUpdates := make(chan time.Duration)
	go startStatsTicker(conn, defaultInterval, intervalUpdates)

	for {
		// Read input from terminal
		fmt.Print("> ")
		text, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		trimmed := strings.TrimSpace(text)

		if strings.HasPrefix(trimmed, "/interval ") {
			ms, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "/interval ")))
			if err != nil || ms <= 0 {
				fmt.Println("❌ Interval must be a positive integer (milliseconds).")
				continue
			}
			intervalUpdates <- time.Duration(ms) * time.Millisecond
			fmt.Printf("⏱️ Stats interval set to %dms\n", ms)
			continue
		}

		// Send message
		_, err = conn.Write([]byte(text))
		if err != nil {
			fmt.Println("❌ Error sending:", err)
			break
		}

		// Read response from server
		response, _ := bufio.NewReader(conn).ReadString('\n')
		fmt.Printf("📨 Server responded: %s", response)
	}
}
