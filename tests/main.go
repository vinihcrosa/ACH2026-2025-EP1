package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

type procInfo struct {
	pid        int32
	name       string
	cpuPercent float64
	memRSSMB   float64
	memPercent float32
}

func main() {
	if err := showProcessesWithGopsutil(); err != nil {
		fmt.Println("⚠️  Não foi possível usar gopsutil:", err)
		fmt.Println("➡️  Usando fallback via comando ps")
		if err := showProcessesWithPS(); err != nil {
			log.Fatalf("Erro ao listar processos via ps: %v", err)
		}
	}
}

func showProcessesWithGopsutil() error {
	procs, err := process.Processes()
	if err != nil {
		return err
	}

	// Prime CPU counters
	for _, p := range procs {
		_, _ = p.CPUPercent()
	}
	time.Sleep(500 * time.Millisecond)

	var infos []procInfo
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

		infos = append(infos, procInfo{
			pid:        p.Pid,
			name:       name,
			cpuPercent: cpuPercent,
			memRSSMB:   float64(memInfo.RSS) / 1024.0 / 1024.0,
			memPercent: memPercent,
		})
	}

	if len(infos) == 0 {
		return fmt.Errorf("nenhuma informação de processo disponível")
	}

	// Ordena por uso de CPU desc
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].cpuPercent > infos[j].cpuPercent
	})

	for _, info := range infos {
		fmt.Printf("PID: %-6d | Nome: %-30s | CPU: %6.2f%% | Mem: %7.2f MB (%5.2f%%)\n",
			info.pid,
			truncate(info.name, 30),
			info.cpuPercent,
			info.memRSSMB,
			info.memPercent,
		)
	}
	return nil
}

func showProcessesWithPS() error {
	// macOS e Linux fornecem ps com estas flags
	cmd := exec.CommandContext(context.Background(), "ps", "-axo", "pid,pcpu,pmem,comm")
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	// pular header
	if scanner.Scan() {
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pidStr := fields[0]
		cpuStr := fields[1]
		memStr := fields[2]
		name := strings.Join(fields[3:], " ")

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		cpuPercent, err := strconv.ParseFloat(cpuStr, 64)
		if err != nil {
			continue
		}
		memPercent, err := strconv.ParseFloat(memStr, 64)
		if err != nil {
			continue
		}

		fmt.Printf("PID: %-6d | Nome: %-30s | CPU: %6.2f%% | Mem: %6.2f%%\n",
			pid,
			truncate(name, 30),
			cpuPercent,
			memPercent,
		)
	}

	return scanner.Err()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
