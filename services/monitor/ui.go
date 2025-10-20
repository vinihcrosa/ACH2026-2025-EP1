package main

import (
	"fmt"
	"libs/protocol"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/rivo/tview"
)

const (
	intervalStepMs = int64(1000)
	minIntervalMs  = int64(500)
	maxIntervalMs  = int64(60000)
)

// monitorUI encapsula os widgets e interações da interface TUI.
type monitorUI struct {
	app       *tview.Application
	list      *tview.List
	details   *tview.TextView
	status    *tview.TextView
	state     monitorState
	selected  string
	conn      net.Conn
	lastError error
}

// newMonitorUI monta a estrutura visual e callbacks básicos da aplicação.
func newMonitorUI(app *tview.Application, conn net.Conn) *monitorUI {
	list := tview.NewList()
	list.ShowSecondaryText(true)
	list.SetWrapAround(true)
	list.SetBorder(true)
	list.SetTitle(" Clientes ")
	list.SetHighlightFullLine(true)

	details := tview.NewTextView()
	details.SetDynamicColors(true)
	details.SetWrap(true)
	details.SetRegions(false)
	details.SetTitle(" Detalhes ")
	details.SetBorder(true)

	status := tview.NewTextView()
	status.SetDynamicColors(true)
	status.SetTextAlign(tview.AlignLeft)
	status.SetBorder(true)
	status.SetTitle(" Status ")

	ui := &monitorUI{
		app:     app,
		list:    list,
		details: details,
		status:  status,
		state:   newMonitorState(),
		conn:    conn,
	}

	list.SetChangedFunc(func(index int, mainText string, secondary string, shortcut rune) {
		ui.selectIndex(index)
	})

	list.SetDoneFunc(func() {
		app.Stop()
	})

	return ui
}

// layout devolve a raiz do layout em colunas e rodapé da interface.
func (ui *monitorUI) layout() tview.Primitive {
	header := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText("🛰️  Monitor - Clientes conectados")

	mainFlex := tview.NewFlex().
		AddItem(ui.list, 32, 0, true).
		AddItem(ui.details, 0, 1, false)

	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(mainFlex, 0, 1, true).
		AddItem(ui.status, 3, 0, false)

	return root
}

// refreshList atualiza a lista lateral cuidando da seleção corrente.
func (ui *monitorUI) refreshList() {
	current := ui.selected
	ui.list.Clear()
	for _, id := range ui.state.order {
		client := ui.state.clients[id]
		name := displayName(client)
		elapsed := "n/d"
		if !client.LastUpdate.IsZero() {
			elapsed = time.Since(client.LastUpdate).Round(time.Second).String()
		}
		ui.list.AddItem(name, fmt.Sprintf("%s | Atualizado há %s", client.RemoteAddr, elapsed), 0, nil)
	}

	if len(ui.state.order) == 0 {
		ui.selected = ""
		ui.details.SetText("Nenhum cliente conectado.\n\nAguardando dados...")
		return
	}

	index := 0
	if current != "" {
		for idx, id := range ui.state.order {
			if id == current {
				index = idx
				break
			}
		}
	}

	ui.list.SetCurrentItem(index)
	ui.selectIndex(index)
}

// renderDetails preenche o painel com os números e gráficos do cliente ativo.
func (ui *monitorUI) renderDetails() {
	if ui.selected == "" {
		ui.details.SetText("Selecione um cliente para ver detalhes.")
		return
	}

	client, ok := ui.state.clients[ui.selected]
	if !ok {
		ui.details.SetText("Dados indisponíveis.")
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[yellow]Cliente:[-] %s\n", displayName(client))
	if client.RemoteAddr != "" {
		fmt.Fprintf(&b, "[yellow]Origem:[-] %s\n", client.RemoteAddr)
	}
	if client.General != nil {
		fmt.Fprintf(&b, "CPU: %s | Cores: %d | %.2f MHz\n",
			client.General.ModelName, client.General.Cores, client.General.Mhz)
	}

	sections := make([]string, 0, 3)

	if client.CPU != nil {
		cpuInfo := []string{fmt.Sprintf("[yellow]CPU Geral:[-] %s %5.1f%%", coloredBar(client.CPU.Usage, 20), client.CPU.Usage)}
		for idx, usage := range client.CPU.CoresUsage {
			cpuInfo = append(cpuInfo, fmt.Sprintf("Core%-2d %s %5.1f%%", idx+1, coloredBar(usage, 16), usage))
		}
		var heat []string
		if hist, ok := ui.state.history[ui.selected]; ok {
			heat = labelledHeatmapLines("CPU (%)", hist.CPU, 26, 12)
		}
		sections = append(sections, mergeColumns(heat, cpuInfo, "   "))
	}

	memInfo := []string{}
	if client.Memory != nil {
		memInfo = append(memInfo, fmt.Sprintf("[yellow]Memória:[-] %s %5.1f%% (%s / %s)",
			coloredBar(client.Memory.UsedPercent, 20),
			client.Memory.UsedPercent,
			humanBytes(client.Memory.Used),
			humanBytes(client.Memory.Total)))
	}
	if client.Disk != nil {
		memInfo = append(memInfo, fmt.Sprintf("[yellow]Disco:[-] %s %5.1f%% (%s / %s)",
			coloredBar(client.Disk.UsedPercent, 20),
			client.Disk.UsedPercent,
			humanBytes(client.Disk.Used),
			humanBytes(client.Disk.Total)))
	}
	if client.StatsIntervalMs > 0 {
		memInfo = append(memInfo, fmt.Sprintf("[yellow]Intervalo de envio:[-] %d ms", client.StatsIntervalMs))
	}
	if len(memInfo) > 0 {
		var heat []string
		if hist, ok := ui.state.history[ui.selected]; ok {
			heat = labelledHeatmapLines("Memória (%)", hist.Memory, 26, 6)
		}
		sections = append(sections, mergeColumns(heat, memInfo, "   "))
	}

	if len(sections) > 0 {
		fmt.Fprintf(&b, "\n%s\n", strings.Join(sections, "\n\n"))
	}

	if client.Processes != nil && len(client.Processes.Processes) > 0 {
		fmt.Fprintf(&b, "\n\n[yellow]Processos (top %d):[-]\n", min(5, len(client.Processes.Processes)))
		procs := append([]protocol.ProcessInfo(nil), client.Processes.Processes...)
		sort.Slice(procs, func(i, j int) bool {
			return procs[i].CPUPercent > procs[j].CPUPercent
		})
		for i := 0; i < min(5, len(procs)); i++ {
			p := procs[i]
			fmt.Fprintf(&b, " %2d. PID=%d %-24s CPU=%6.2f%% MEM=%8.2fMB\n",
				i+1, p.PID, truncate(p.Name, 24), p.CPUPercent, p.MemoryMB)
		}
	}

	ui.details.SetText(b.String())
}

// selectIndex movimenta a seleção e refaz o painel de detalhes.
func (ui *monitorUI) selectIndex(index int) {
	if index < 0 || index >= len(ui.state.order) {
		ui.selected = ""
		ui.details.SetText("Selecione um cliente para ver detalhes.")
		return
	}
	ui.selected = ui.state.order[index]
	ui.renderDetails()
}

// setStatus escreve uma mensagem no rodapé da interface.
func (ui *monitorUI) setStatus(msg string) {
	ui.status.SetText(msg)
}

// changeSelectedInterval ajusta o intervalo de telemetria do cliente ativo.
func (ui *monitorUI) changeSelectedInterval(deltaMs int64) {
	if ui.selected == "" {
		ui.setStatus("Selecione um cliente antes de ajustar o intervalo.")
		return
	}

	client, ok := ui.state.clients[ui.selected]
	if !ok {
		ui.setStatus("Cliente não encontrado.")
		return
	}

	current := client.StatsIntervalMs
	if current == 0 {
		current = 5000
	}

	newValue := current + deltaMs
	if newValue < minIntervalMs {
		newValue = minIntervalMs
	}
	if newValue > maxIntervalMs {
		newValue = maxIntervalMs
	}
	if newValue == current {
		if deltaMs < 0 {
			ui.setStatus("Intervalo já está no mínimo permitido.")
		} else if deltaMs > 0 {
			ui.setStatus("Intervalo já está no máximo permitido.")
		}
		return
	}

	clientID := clientIDFromSummary(client)
	if clientID == "" {
		ui.setStatus("Cliente não possui ID válido.")
		return
	}

	if err := sendIntervalSetRequest(ui.conn, clientID, newValue); err != nil {
		ui.setStatus(fmt.Sprintf("[red]Erro ao enviar novo intervalo: %v", err))
		return
	}

	ui.setStatus(fmt.Sprintf("Solicitado novo intervalo (%d ms) para %s", newValue, displayName(client)))
}

// displayName decide qual identificador deve aparecer na UI.
func displayName(client protocol.ClientStateSummary) string {
	if client.Handshake != nil && client.Handshake.ClientID != "" {
		return client.Handshake.ClientID
	}
	return client.RemoteAddr
}

// Código gerado com auxílio de IA.

// clientIDFromSummary devolve o identificador usado para enviar comandos ao cliente.
func clientIDFromSummary(client protocol.ClientStateSummary) string {
	if client.Handshake != nil && client.Handshake.ClientID != "" {
		return client.Handshake.ClientID
	}
	return client.RemoteAddr
}
