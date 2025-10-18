package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"libs/protocol"
	"libs/utils"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type monitorState struct {
	clients map[string]protocol.ClientStateSummary
	order   []string
}

func newMonitorState() monitorState {
	return monitorState{
		clients: make(map[string]protocol.ClientStateSummary),
		order:   []string{},
	}
}

func clientKey(summary protocol.ClientStateSummary) string {
	if summary.Handshake != nil && summary.Handshake.ClientID != "" {
		return summary.Handshake.ClientID
	}
	return summary.RemoteAddr
}

func (s *monitorState) applySnapshot(list []protocol.ClientStateSummary) {
	s.clients = make(map[string]protocol.ClientStateSummary, len(list))
	s.order = s.order[:0]
	for _, item := range list {
		id := clientKey(item)
		s.clients[id] = item
		s.order = append(s.order, id)
	}
	sort.Strings(s.order)
}

func (s *monitorState) applyUpdate(summary protocol.ClientStateSummary) {
	id := clientKey(summary)
	if _, exists := s.clients[id]; !exists {
		s.order = append(s.order, id)
		sort.Strings(s.order)
	}
	s.clients[id] = summary
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
		if index >= 0 && index < len(ui.state.order) {
			ui.selected = ui.state.order[index]
			ui.renderDetails()
		}
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
		ui.list.AddItem(name, fmt.Sprintf("Atualizado há %s", elapsed), 0, nil)
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
	ui.selected = ui.state.order[index]
	ui.renderDetails()
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
	if client.General != nil {
		fmt.Fprintf(&b, "CPU: %s | Cores: %d | %.2f MHz\n",
			client.General.ModelName, client.General.Cores, client.General.Mhz)
	}

	if client.CPU != nil {
		fmt.Fprintf(&b, "\n[yellow]Uso de CPU:[-] %.2f%%", client.CPU.Usage)
		if len(client.CPU.CoresUsage) > 0 {
			fmt.Fprintf(&b, " | Núcleos: %v", shortSlice(client.CPU.CoresUsage, 6))
		}
	}
	if client.Memory != nil {
		fmt.Fprintf(&b, "\n[yellow]Uso de Memória:[-] %.2f%% (%s / %s)",
			client.Memory.UsedPercent,
			humanBytes(client.Memory.Used),
			humanBytes(client.Memory.Total))
	}
	if client.Disk != nil {
		fmt.Fprintf(&b, "\n[yellow]Uso de Disco:[-] %.2f%% (%s / %s)",
			client.Disk.UsedPercent,
			humanBytes(client.Disk.Used),
			humanBytes(client.Disk.Total))
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

func (ui *monitorUI) setStatus(msg string) {
	ui.status.SetText(msg)
}

func displayName(client protocol.ClientStateSummary) string {
	if client.Handshake != nil && client.Handshake.ClientID != "" {
		return client.Handshake.ClientID
	}
	return client.RemoteAddr
}

func shortSlice(values []float64, max int) string {
	n := min(len(values), max)
	if n == 0 {
		return "[]"
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fmt.Sprintf("%.1f%%", values[i])
	}
	if n < len(values) {
		parts = append(parts, "…")
	}
	return "[" + strings.Join(parts, ", ") + "]"
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
	conn, err := net.Dial("tcp", "localhost:8080")
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
