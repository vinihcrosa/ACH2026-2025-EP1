package protocol

import "time"

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type HandshakeData struct {
	ClientID string `json:"client_id"`
	Version  string `json:"version"`
	Role     string `json:"role"`
}

type CpuUsageData struct {
	Usage      float64   `json:"usage"`
	CoresUsage []float64 `json:"cores_usage"`
}

type MemoryUsageData struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

type DiskUsageData struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type GeneralData struct {
	ModelName string  `json:"model_name"`
	Cores     int32   `json:"cores"`
	Mhz       float64 `json:"mhz"`
}

type ProcessUsageData struct {
	Processes []ProcessInfo `json:"processes"`
}

type ProcessInfo struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryMB      float64 `json:"memory_mb"`
	MemoryPercent float32 `json:"memory_percent"`
}

type ClientsRequestData struct{}

type ClientStateSummary struct {
	RemoteAddr string            `json:"remote_addr"`
	Handshake  *HandshakeData    `json:"handshake,omitempty"`
	CPU        *CpuUsageData     `json:"cpu,omitempty"`
	Memory     *MemoryUsageData  `json:"memory,omitempty"`
	Disk       *DiskUsageData    `json:"disk,omitempty"`
	General    *GeneralData      `json:"general,omitempty"`
	Processes  *ProcessUsageData `json:"processes,omitempty"`
	LastUpdate time.Time         `json:"last_update"`
}

type ClientsStateData struct {
	Clients     []ClientStateSummary `json:"clients"`
	GeneratedAt time.Time            `json:"generated_at"`
}

type ClientUpdateData struct {
	Client ClientStateSummary `json:"client"`
}

type ClientRemovedData struct {
	ClientID string `json:"client_id"`
}
