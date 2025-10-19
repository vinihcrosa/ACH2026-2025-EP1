package main

import (
	"fmt"
	"libs/protocol"
	"net"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// runMonitor configura a conexão com o servidor e inicializa a interface TUI.
func runMonitor(address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("não foi possível conectar ao servidor %s: %w", address, err)
	}
	defer conn.Close()

	if err := sendMonitorHandshake(conn); err != nil {
		return fmt.Errorf("falha ao enviar handshake: %w", err)
	}

	if err := sendClientsRequest(conn); err != nil {
		return fmt.Errorf("falha ao solicitar lista de clientes: %w", err)
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
		return fmt.Errorf("erro ao executar interface: %w", err)
	}

	return nil
}
