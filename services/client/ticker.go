package main

import (
	"fmt"
	"net"
	"time"
)

func startCpuTicker(conn net.Conn, initial time.Duration, updates <-chan time.Duration) {
	ticker := time.NewTicker(initial)
	defer ticker.Stop()

	if err := sendCpuUsage(conn); err != nil {
		fmt.Println("❌ Error sending CPU usage:", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := sendCpuUsage(conn); err != nil {
				fmt.Println("❌ Error sending CPU usage:", err)
			}
		case next, ok := <-updates:
			if !ok {
				return
			}
			if next <= 0 {
				fmt.Println("❌ Interval must be greater than zero.")
				continue
			}
			ticker.Stop()
			ticker = time.NewTicker(next)
		}
	}
}
