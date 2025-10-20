package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"libs/protocol"
	"libs/utils"
	"net"
)

// sendMonitorHandshake identifica a conexão atual como monitor para o servidor.
func sendMonitorHandshake(conn net.Conn) error {
	msg := protocol.Message{
		Type: "handshake",
		Data: protocol.HandshakeData{
			ClientID: "monitor",
			Version:  "1.0.0",
			Role:     "monitor",
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(payload, '\n'))
	return err
}

// sendClientsRequest solicita ao servidor o snapshot completo dos clientes.
func sendClientsRequest(conn net.Conn) error {
	msg := protocol.Message{
		Type: "clients_request",
		Data: protocol.ClientsRequestData{},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(payload, '\n'))
	return err
}

// sendIntervalSetRequest pede para o servidor reajustar o intervalo de métricas.
func sendIntervalSetRequest(conn net.Conn, clientID string, intervalMs int64) error {
	msg := protocol.Message{
		Type: "interval_set_request",
		Data: protocol.IntervalUpdateData{
			ClientID:   clientID,
			IntervalMs: intervalMs,
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(payload, '\n'))
	return err
}

// listenServer fica lendo a conexão e roteando mensagens para os canais corretos.
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
			// mensagens desconhecidas são ignoradas
		}
	}
}

// Código gerado com auxílio de IA.
