package main

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

func main() {
	// 🧠 CPU
	percent, _ := cpu.Percent(time.Second, false) // false = total, true = por núcleo
	fmt.Printf("💻 Uso de CPU: %.2f%%\n", percent[0])

	// 💾 Memória
	vmStat, _ := mem.VirtualMemory()
	fmt.Printf("📊 Memória usada: %.2f GB (%.2f%%)\n", float64(vmStat.Used)/1e9, vmStat.UsedPercent)

	// 💽 Disco
	usage, _ := disk.Usage("/")
	fmt.Printf("📁 Disco usado: %.2f GB de %.2f GB (%.2f%%)\n",
		float64(usage.Used)/1e9, float64(usage.Total)/1e9, usage.UsedPercent)

	cpuStats, _ := cpu.Info()
	if len(cpuStats) > 0 {
		fmt.Printf("🖥️ CPU: %s, Cores: %d, Velocidade: %.2f MHz\n",
			cpuStats[0].ModelName, cpuStats[0].Cores, cpuStats[0].Mhz)
	}
}
