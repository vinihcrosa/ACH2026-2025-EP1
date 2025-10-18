package main

import (
	"errors"
	"fmt"
	"net"
	"time"
)

const processSampleSize = 10

func startStatsTicker(conn net.Conn, initial time.Duration, updates <-chan time.Duration) {
	ticker := time.NewTicker(initial)
	defer ticker.Stop()

	if err := sendAllStats(conn); err != nil {
		fmt.Println("❌ Error sending stats:", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := sendAllStats(conn); err != nil {
				fmt.Println("❌ Error sending stats:", err)
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

func sendAllStats(conn net.Conn) error {
	var errs []error

	if err := sendCpuUsage(conn); err != nil {
		errs = append(errs, fmt.Errorf("cpu usage: %w", err))
	}
	if err := sendMemoryUsage(conn); err != nil {
		errs = append(errs, fmt.Errorf("memory usage: %w", err))
	}
	if err := sendDiskUsage(conn); err != nil {
		errs = append(errs, fmt.Errorf("disk usage: %w", err))
	}
	if err := sendProcessUsage(conn, processSampleSize); err != nil {
		errs = append(errs, fmt.Errorf("process usage: %w", err))
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}
