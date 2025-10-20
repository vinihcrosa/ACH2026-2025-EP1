package main

import (
	"libs/protocol"
	"sort"
)

const historyCapacity = 60

// monitorState agrega as últimas métricas recebidas de cada cliente conhecido.
type monitorState struct {
	clients map[string]protocol.ClientStateSummary
	order   []string
	history map[string]*statsHistory
}

// statsHistory guarda séries históricas usadas para os gráficos de calor.
type statsHistory struct {
	CPU    []float64
	Memory []float64
}

// newMonitorState cria uma instância pronta para uso do estado compartilhado.
func newMonitorState() monitorState {
	return monitorState{
		clients: make(map[string]protocol.ClientStateSummary),
		order:   []string{},
		history: make(map[string]*statsHistory),
	}
}

// clientKey determina a chave estável para um cliente (ID ou endereço remoto).
func clientKey(summary protocol.ClientStateSummary) string {
	if summary.Handshake != nil && summary.Handshake.ClientID != "" {
		return summary.Handshake.ClientID
	}
	return summary.RemoteAddr
}

// applySnapshot substitui o estado atual pelos dados recebidos em massa.
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

// applyUpdate injeta novas métricas incrementais para um cliente específico.
func (s *monitorState) applyUpdate(summary protocol.ClientStateSummary) {
	id := clientKey(summary)
	if _, exists := s.clients[id]; !exists {
		s.order = append(s.order, id)
		sort.Strings(s.order)
	}

	s.clients[id] = summary
	s.appendMetrics(id, summary)
}

// applyRemoval remove as informações de um cliente desconectado.
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

// appendMetrics atualiza as séries históricas com os novos valores.
func (s *monitorState) appendMetrics(id string, summary protocol.ClientStateSummary) {
	h := s.ensureHistory(id)
	if summary.CPU != nil {
		h.CPU = appendValue(h.CPU, summary.CPU.Usage)
	}
	if summary.Memory != nil {
		h.Memory = appendValue(h.Memory, summary.Memory.UsedPercent)
	}
}

// ensureHistory devolve (criando se necessário) a série histórica de um cliente.
func (s *monitorState) ensureHistory(id string) *statsHistory {
	if h, ok := s.history[id]; ok {
		return h
	}
	h := &statsHistory{}
	s.history[id] = h
	return h
}

// appendValue acrescenta um dado na série mantendo o tamanho máximo configurado.
func appendValue(values []float64, v float64) []float64 {
	values = append(values, v)
	if len(values) > historyCapacity {
		start := len(values) - historyCapacity
		copy(values, values[start:])
		values = values[:historyCapacity]
	}
	return values
}

// Código gerado com auxílio de IA.
