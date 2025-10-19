# Protocolo de Mensagens

Este documento descreve todas as mensagens trafegadas entre os processos `client`, `server` e `monitor`. Cada mensagem é enviada como uma linha JSON (sufixo `\n`) obedecendo ao struct `protocol.Message`:

```json
{ "type": "<tipo>", "data": { ... } }
```

Todas as conexões são iniciadas via TCP. O emissor deve enviar um *handshake* antes de qualquer outra mensagem para que o servidor determine o papel da conexão.

## Papéis

- **Client**: coleta métricas locais e as envia ao servidor. Também aceita comandos de ajuste de intervalo vindos do servidor.
- **Monitor**: pede snapshots e emite solicitações administrativas (por exemplo, ajuste de intervalo de um cliente). Recebe atualizações em tempo real do servidor.
- **Server**: nodo central que coordena o armazenamento do estado, retransmite updates aos monitores e encaminha comandos aos clientes.

## Mensagens por origem

### Client → Server

| Tipo                | Payload (`data`)                  | Descrição |
| ------------------ | --------------------------------- | --------- |
| `handshake`        | `HandshakeData`                   | Informações do cliente (`client_id`, versão, `role="client"`). |
| `cpu_usage`        | `CpuUsageData`                    | Percentual médio da CPU e por núcleo. |
| `memory_usage`     | `MemoryUsageData`                 | Uso atual de memória RAM. |
| `disk_usage`       | `DiskUsageData`                   | Uso do volume raiz. |
| `general_data`     | `GeneralData`                     | Informações estáticas da CPU. |
| `process_usage`    | `ProcessUsageData`                | Lista dos processos monitorados. |
| `interval_update`  | `IntervalUpdateData`              | Confirmação do intervalo de envio atual (em milissegundos). |

### Monitor → Server

| Tipo                   | Payload (`data`)         | Descrição |
| --------------------- | ------------------------ | --------- |
| `handshake`           | `HandshakeData`          | Identifica a conexão (`role="monitor"`). |
| `clients_request`     | `ClientsRequestData{}`   | Solicita snapshot completo dos clientes. |
| `interval_set_request`| `IntervalUpdateData`     | Pede alteração do intervalo de um cliente específico (`client_id`, `interval_ms`). |

### Server → Client

| Tipo           | Payload (`data`)        | Descrição |
| -------------- | ----------------------- | --------- |
| `set_interval` | `IntervalUpdateData`    | Comando para o cliente ajustar o intervalo de envio. Sem `client_id`; apenas `interval_ms`. |

### Server → Monitor

| Tipo             | Payload (`data`)        | Descrição |
| ---------------- | ----------------------- | --------- |
| `clients_state`  | `ClientsStateData`      | Snapshot completo de todos os clientes (`clients`, `generated_at`). |
| `client_update`  | `ClientUpdateData`      | Atualização incremental do estado de um cliente. Inclui `stats_interval_ms`. |
| `client_removed` | `ClientRemovedData`     | Notificação de desconexão (`client_id`). |

### Notas gerais

- **Intervalos**: todos os valores são trocados em milissegundos (`interval_ms`). O cliente envia um `interval_update` tanto ao iniciar quanto ao receber um novo intervalo; o servidor usa esse dado para atualizar o estado que repassa aos monitores.
- **Persistência em memória**: o servidor mantém para cada cliente o último snapshot de todas as métricas, bem como o intervalo atual. Esses dados são copiados para os monitores em forma de `ClientStateSummary`.
- **Mensagens desconhecidas**: o servidor ignora mensagens cujo `type` não esteja autorizado para o papel registrado durante o handshake.

## Fluxo típico

1. O cliente conecta e envia `handshake`. O servidor reconhece e passa a aceitar as demais mensagens.
2. O cliente coleta métricas periodicamente e envia `cpu_usage`, `memory_usage`, `disk_usage`, `general_data` e `process_usage`.
3. O servidor atualiza o estado em memória e retransmite `client_update` para todos os monitores conectados.
4. O monitor pode solicitar a lista completa (`clients_request`) ou ajustar o intervalo de um cliente (`interval_set_request`).
5. Ao ajustar um intervalo, o servidor envia `set_interval` ao cliente correspondente. O cliente aplica, responde com `interval_update` e continua enviando métricas no novo ritmo.
6. Se o cliente desconectar, o servidor remove seu estado e envia `client_removed` aos monitores.

Esta especificação reflete a implementação presente na pasta `libs/protocol` e nos serviços `client`, `server` e `monitor`.

