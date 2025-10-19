package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var writeMu sync.Mutex

func main() {
	host := flag.String("host", "localhost", "Server host or IP")
	port := flag.Int("port", 8080, "Server TCP port")
	clientID := flag.String("id", "client", "Client identifier for handshake")
	flag.Parse()

	address := fmt.Sprintf("%s:%d", *host, *port)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Printf("✅ Connected to %s. Type messages:\n", address)

	// Send handshake message
	err = sendHandshake(conn, *clientID)
	if err != nil {
		fmt.Println("❌ Error sending handshake:", err)
		return
	}
	if err := sendGeneralData(conn); err != nil {
		fmt.Println("❌ Error sending general data:", err)
	}

	defaultInterval := 5 * time.Second
	intervalUpdates := make(chan time.Duration, 1)

	var (
		intervalMu      sync.Mutex
		currentInterval = defaultInterval
	)

	setInterval := func(newInterval time.Duration, source string, notify bool) {
		if newInterval <= 0 {
			fmt.Println("❌ Interval must be greater than zero.")
			return
		}
		intervalMu.Lock()
		previous := currentInterval
		currentInterval = newInterval
		intervalMu.Unlock()
		if previous != newInterval {
			select {
			case intervalUpdates <- newInterval:
			default:
				select {
				case <-intervalUpdates:
				default:
				}
				intervalUpdates <- newInterval
			}
		}
		fmt.Printf("⏱️ Stats interval set to %dms (%s)\n", newInterval.Milliseconds(), source)
		if notify {
			if err := sendIntervalUpdate(conn, newInterval); err != nil {
				fmt.Println("❌ Error notifying interval update:", err)
			}
		}
	}

	go startStatsTicker(conn, defaultInterval, intervalUpdates)

    go listenServer(conn, func(interval time.Duration) {
        setInterval(interval, "servidor", true)
    }, func(err error) {
        fmt.Println("❌ Connection closed by server:", err)
        os.Exit(1)
    })

	if err := sendIntervalUpdate(conn, currentInterval); err != nil {
		fmt.Println("⚠️ Could not notify initial interval:", err)
	}

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
			setInterval(time.Duration(ms)*time.Millisecond, "comando local", true)
			continue
		}

		// Send message
		writeMu.Lock()
		_, err = conn.Write([]byte(text))
		writeMu.Unlock()
		if err != nil {
			fmt.Println("❌ Error sending:", err)
			break
		}
	}
}
