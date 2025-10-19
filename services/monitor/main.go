package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"libs/protocol"
	"libs/utils"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

type monitorState struct {
	clients map[string]protocol.ClientStateSummary
	order   []string
	history map[string]*statsHistory
}

const (
	intervalStepMs  = int64(1000)
	minIntervalMs   = int64(500)
	maxIntervalMs   = int64(60000)
	historyCapacity = 60
)

type statsHistory struct {
	CPU    []float64
	Memory []float64
}

func newMonitorState() monitorState {
	return monitorState{
		clients: make(map[string]protocol.ClientStateSummary),
		order:   []string{},
		history: make(map[string]*statsHistory),
	}
}

func clientKey(summary protocol.ClientStateSummary) string {
	if summary.Handshake != nil && summary.Handshake.ClientID != "" {
		return summary.Handshake.ClientID
	}
	return summary.RemoteAddr
}

func (s *monitorState) applySnapshot(list []protocol.ClientStateSummary) {
	active := make(map[string]struct{}, len(list))
	newClients := make(map[string]protocol.ClientStateSummary, len(list))
	newOrder := make([]string, 0, len(list))
	for _, item := range list {
		id := clientKey(item)
		newClients[id] = item
		newOrder = append(newOrder, id)
		active[id] = struct{}{}
		s.appendMetrics(id, item)
	}
	sort.Strings(newOrder)
	s.clients = newClients
	s.order = newOrder

	for id := range s.history {
		if _, ok := active[id]; !ok {
			delete(s.history, id)
		}
	}
}

func (s *monitorState) applyUpdate(summary protocol.ClientStateSummary) {
	id := clientKey(summary)
	if _, exists := s.clients[id]; !exists {
		s.order = append(s.order, id)
		sort.Strings(s.order)
	}
	s.clients[id] = summary
	s.appendMetrics(id, summary)
}

func (s *monitorState) applyRemoval(clientID string) {
	if clientID == "" {
		return
	}
	delete(s.clients, clientID)
	for idx, id := range s.order {
		if id == clientID {
			s.order = append(s.order[:idx], s.order[idx+1:]...)
			break
		}
	}
	delete(s.history, clientID)
}

func (s *monitorState) appendMetrics(id string, summary protocol.ClientStateSummary) {
	h := s.ensureHistory(id)
	if summary.CPU != nil {
		h.CPU = appendValue(h.CPU, summary.CPU.Usage)
	}
	if summary.Memory != nil {
		h.Memory = appendValue(h.Memory, summary.Memory.UsedPercent)
	}
}

func (s *monitorState) ensureHistory(id string) *statsHistory {
	if h, ok := s.history[id]; ok {
		return h
	}
	h := &statsHistory{}
	s.history[id] = h
	return h
}

func appendValue(values []float64, v float64) []float64 {
	values = append(values, v)
	if len(values) > historyCapacity {
		start := len(values) - historyCapacity
		copy(values, values[start:])
		values = values[:historyCapacity]
	}
	return values
}

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

func (ui *monitorUI) selectIndex(index int) {
	if index < 0 || index >= len(ui.state.order) {
		ui.selected = ""
		ui.details.SetText("Selecione um cliente para ver detalhes.")
		return
	}
	ui.selected = ui.state.order[index]
	ui.renderDetails()
}

func (ui *monitorUI) setStatus(msg string) {
	ui.status.SetText(msg)
}

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

func displayName(client protocol.ClientStateSummary) string {
	if client.Handshake != nil && client.Handshake.ClientID != "" {
		return client.Handshake.ClientID
	}
	return client.RemoteAddr
}

func clientIDFromSummary(client protocol.ClientStateSummary) string {
	if client.Handshake != nil && client.Handshake.ClientID != "" {
		return client.Handshake.ClientID
	}
	return client.RemoteAddr
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func humanBytes(v uint64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%dB", v)
	}
	div, exp := uint64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(v) / float64(div)
	return fmt.Sprintf("%.2f%cB", value, "KMGTPE"[exp])
}

func coloredBar(value float64, width int) string {
	if width <= 0 {
		width = 1
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	filled := int(math.Round(value / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	color := colorForUsage(value)
	var b strings.Builder
	if filled > 0 {
		b.WriteString("[")
		b.WriteString(color)
		b.WriteString("]")
		b.WriteString(strings.Repeat("█", filled))
		b.WriteString("[-]")
	}
	if width-filled > 0 {
		b.WriteString("[#3b3b3b]")
		b.WriteString(strings.Repeat("░", width-filled))
		b.WriteString("[-]")
	}
	return b.String()
}

func colorForUsage(value float64) string {
	switch {
	case value >= 90:
		return "#ff5555"
	case value >= 75:
		return "#ffb86c"
	case value >= 50:
		return "#f1fa8c"
	case value >= 25:
		return "#50fa7b"
	default:
		return "#8be9fd"
	}
}

func labelledHeatmapLines(title string, values []float64, width, height int) []string {
	lines := renderHeatmapLines(values, width, height)
	if len(lines) == 0 {
		return nil
	}
	label := fmt.Sprintf("[yellow]%s[-]", title)
	if len(label) > 0 {
		lines[0] = padMarkup(label, visibleWidth(lines[0]))
	}
	return lines
}

func renderHeatmapLines(values []float64, width, height int) []string {
	if len(values) == 0 || height <= 0 {
		return nil
	}
	if width <= 0 {
		width = len(values)
	}
	if len(values) > width {
		values = values[len(values)-width:]
	} else if len(values) < width {
		padding := make([]float64, width-len(values))
		values = append(padding, values...)
	}
	if height < 2 {
		height = 2
	}

	lines := make([]string, height)
	for row := height - 1; row >= 0; row-- {
		var line strings.Builder
		for _, v := range values {
			filled := int(math.Round(v / 100 * float64(height-1)))
			if filled > height-1 {
				filled = height - 1
			}
			if filled < 0 {
				filled = 0
			}
			if filled >= row {
				color := colorForUsage(v)
				line.WriteString("[")
				line.WriteString(color)
				line.WriteString("]•[-]")
			} else {
				line.WriteString(" ")
			}
		}
		lines[height-1-row] = line.String()
	}
	return lines
}

func mergeColumns(left []string, right []string, gap string) string {
	if len(left) == 0 {
		return strings.Join(right, "\n")
	}
	if len(right) == 0 {
		return strings.Join(left, "\n")
	}

	maxWidth := 0
	for _, line := range left {
		if w := visibleWidth(line); w > maxWidth {
			maxWidth = w
		}
	}

	rows := max(len(left), len(right))
	result := make([]string, rows)
	for i := 0; i < rows; i++ {
		var l, r string
		if i < len(left) {
			l = padMarkup(left[i], maxWidth)
		} else {
			l = strings.Repeat(" ", maxWidth)
		}
		if i < len(right) {
			r = right[i]
		}
		result[i] = l + gap + r
	}
	return strings.Join(result, "\n")
}

func indentBlock(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func stripMarkup(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	skip := 0
	for _, r := range text {
		if skip > 0 {
			if r == ']' {
				skip--
			}
			continue
		}
		if r == '[' {
			skip++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func visibleWidth(text string) int {
	stripped := stripMarkup(text)
	return runewidth.StringWidth(stripped)
}

func padMarkup(text string, width int) string {
	diff := width - visibleWidth(text)
	if diff <= 0 {
		return text
	}
	return text + strings.Repeat(" ", diff)
}

func sendMonitorHandshake(conn net.Conn) error {
	msg := protocol.Message{
		Type: "handshake",
		Data: protocol.HandshakeData{
			ClientID: "monitor",
			Version:  "1.0.0",
			Role:     "monitor",
		},
	}

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}

func sendClientsRequest(conn net.Conn) error {
	req := protocol.Message{
		Type: "clients_request",
		Data: protocol.ClientsRequestData{},
	}
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(jsonBytes, '\n'))
	return err
}

func listenServer(conn net.Conn, snapshots chan<- []protocol.ClientStateSummary, updates chan<- protocol.ClientStateSummary, removals chan<- string, errs chan<- error) {
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			errs <- err
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Println("❌ Erro ao decodificar mensagem do servidor:", err)
			continue
		}

		switch msg.Type {
		case "clients_state":
			var data protocol.ClientsStateData
			if err := utils.ParseData(msg.Data, &data); err != nil {
				fmt.Println("❌ Erro ao interpretar clients_state:", err)
				continue
			}
			snapshots <- data.Clients
		case "client_update":
			var data protocol.ClientUpdateData
			if err := utils.ParseData(msg.Data, &data); err != nil {
				fmt.Println("❌ Erro ao interpretar client_update:", err)
				continue
			}
			updates <- data.Client
		case "client_removed":
			var data protocol.ClientRemovedData
			if err := utils.ParseData(msg.Data, &data); err != nil {
				fmt.Println("❌ Erro ao interpretar client_removed:", err)
				continue
			}
			removals <- data.ClientID
		default:
			// outras mensagens são ignoradas
		}
	}
}

func main() {
	host := flag.String("host", "localhost", "Server host or IP")
	port := flag.Int("port", 8080, "Server TCP port")
	flag.Parse()

	address := fmt.Sprintf("%s:%d", *host, *port)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Println("❌ Não foi possível conectar ao servidor:", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := sendMonitorHandshake(conn); err != nil {
		fmt.Println("❌ Falha ao enviar handshake:", err)
		return
	}

	if err := sendClientsRequest(conn); err != nil {
		fmt.Println("❌ Falha ao solicitar lista de clientes:", err)
		return
	}

	app := tview.NewApplication()
	ui := newMonitorUI(app, conn)
	ui.setStatus("Conectado. Use ↑/↓ para navegar, [::b]r[::-] para atualizar, [::b]q[::-]/Esc para sair.")

	snapshotCh := make(chan []protocol.ClientStateSummary, 1)
	updateCh := make(chan protocol.ClientStateSummary, 16)
	removeCh := make(chan string, 16)
	errCh := make(chan error, 1)

	go listenServer(conn, snapshotCh, updateCh, removeCh, errCh)

	go func() {
		for {
			select {
			case list := <-snapshotCh:
				app.QueueUpdateDraw(func() {
					ui.state.applySnapshot(list)
					ui.refreshList()
					if len(list) == 0 {
						ui.setStatus("Nenhum cliente conectado no momento.")
					} else {
						ui.setStatus(fmt.Sprintf("%d cliente(s) conectados. ↑/↓ para navegar, r para atualizar.", len(list)))
					}
				})
			case update := <-updateCh:
				app.QueueUpdateDraw(func() {
					ui.state.applyUpdate(update)
					ui.refreshList()
				})
			case removed := <-removeCh:
				app.QueueUpdateDraw(func() {
					ui.state.applyRemoval(removed)
					ui.refreshList()
					ui.setStatus(fmt.Sprintf("Cliente %s desconectou.", removed))
				})
			case err := <-errCh:
				app.QueueUpdateDraw(func() {
					ui.setStatus(fmt.Sprintf("[red]Conexão encerrada: %v", err))
					app.Stop()
				})
				return
			}
		}
	}()

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyRune && (event.Rune() == 'q' || event.Rune() == 'Q'):
			app.Stop()
			return nil
		case event.Key() == tcell.KeyRune && (event.Rune() == 'r' || event.Rune() == 'R'):
			go func() {
				if err := sendClientsRequest(conn); err != nil {
					app.QueueUpdateDraw(func() {
						ui.setStatus(fmt.Sprintf("[red]Erro ao solicitar atualização: %v", err))
					})
				} else {
					app.QueueUpdateDraw(func() {
						ui.setStatus("Solicitando snapshot ao servidor...")
					})
				}
			}()
			return nil
		case event.Key() == tcell.KeyRune && (event.Rune() == '+' || event.Rune() == '='):
			ui.changeSelectedInterval(-intervalStepMs)
			return nil
		case event.Key() == tcell.KeyRune && (event.Rune() == '-' || event.Rune() == '_'):
			ui.changeSelectedInterval(intervalStepMs)
			return nil
		case event.Key() == tcell.KeyEscape:
			app.Stop()
			return nil
		}
		return event
	})

	if err := app.SetRoot(ui.layout(), true).EnableMouse(true).Run(); err != nil {
		fmt.Println("❌ Erro na UI:", err)
	}
}

func sendIntervalSetRequest(conn net.Conn, clientID string, intervalMs int64) error {
	msg := protocol.Message{
		Type: "interval_set_request",
		Data: protocol.IntervalUpdateData{
			ClientID:   clientID,
			IntervalMs: intervalMs,
		},
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(bytes, '\n'))
	return err
}
