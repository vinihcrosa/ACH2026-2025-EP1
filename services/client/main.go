package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"libs/protocol"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
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
	go startCpuTicker(conn, defaultInterval, intervalUpdates)

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
			fmt.Printf("⏱️ CPU usage interval set to %dms\n", ms)
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

func sendHandshake(conn net.Conn) error {
	msg := protocol.Message{
		Type: "handshake",
		Data: protocol.HandshakeData{
			ClientID: "client123",
			Version:  "1.0.0",
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}

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

func sendCpuUsage(conn net.Conn) error {
	cpuPercent, err := cpu.Percent(time.Second, false)
	coresPercent, err := cpu.Percent(time.Second, true)
	if err != nil {
		return err
	}
	if len(cpuPercent) == 0 {
		return fmt.Errorf("no CPU usage data")
	}
	msg := protocol.Message{
		Type: "cpu_usage",
		Data: protocol.CpuUsageData{
			Usage:      cpuPercent[0],
			CoresUsage: coresPercent,
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}

func sendMemoryUsage(conn net.Conn) error {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return err
	}

	msg := protocol.Message{
		Type: "memory_usage",
		Data: protocol.MemoryUsageData{
			Total:       vmStat.Total,
			Used:        vmStat.Used,
			UsedPercent: vmStat.UsedPercent,
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}

func sendDiskUsage(conn net.Conn) error {
	usage, _ := disk.Usage("/")

	msg := protocol.Message{
		Type: "disk_usage",
		Data: protocol.DiskUsageData{
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}

func sendGeneralData(conn net.Conn) error {
	cpuStats, err := cpu.Info()
	if err != nil || len(cpuStats) == 0 {
		return fmt.Errorf("failed to get CPU info")
	}

	msg := protocol.Message{
		Type: "general_data",
		Data: protocol.GeneralData{
			ModelName: cpuStats[0].ModelName,
			Cores:     cpuStats[0].Cores,
			Mhz:       cpuStats[0].Mhz,
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}
