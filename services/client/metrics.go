package main

import (
	"encoding/json"
	"fmt"
	"libs/protocol"
	"net"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

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

func sendProcessUsage(conn net.Conn, maxEntries int) error {
	if maxEntries <= 0 {
		maxEntries = 10
	}

	procs, err := process.Processes()
	if err != nil {
		return err
	}

	// Prime CPU counters before measuring to avoid zeroed data.
	for _, p := range procs {
		_, _ = p.CPUPercent()
	}
	time.Sleep(200 * time.Millisecond)

	var infos []protocol.ProcessInfo
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}

		cpuPercent, err := p.CPUPercent()
		if err != nil {
			continue
		}

		memInfo, err := p.MemoryInfo()
		if err != nil || memInfo == nil {
			continue
		}

		memPercent, err := p.MemoryPercent()
		if err != nil {
			continue
		}

		infos = append(infos, protocol.ProcessInfo{
			PID:           p.Pid,
			Name:          name,
			CPUPercent:    cpuPercent,
			MemoryMB:      float64(memInfo.RSS) / 1024.0 / 1024.0,
			MemoryPercent: memPercent,
		})
	}

	if len(infos) == 0 {
		return fmt.Errorf("no process usage data collected")
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].CPUPercent > infos[j].CPUPercent
	})

	if len(infos) > maxEntries {
		infos = infos[:maxEntries]
	}

	msg := protocol.Message{
		Type: "process_usage",
		Data: protocol.ProcessUsageData{
			Processes: infos,
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}
