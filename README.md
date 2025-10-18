# Monitoramento Distribuído via Socket TCP

Este projeto implementa um pequeno sistema de monitoramento usando sockets TCP em Go. Há dois serviços principais:

- `services/server`: servidor que aceita conexões e interpreta mensagens estruturadas em JSON.
- `services/client`: cliente que coleta métricas locais (CPU, memória, disco, informações gerais) e envia dados ao servidor.

A pasta `libs/` concentra o protocolo de mensagens (`libs/protocol`) e utilidades para o parsing de dados (`libs/utils`).

---

## Comunicação via Socket

- **Transporte:** TCP (`net.Dial` no cliente e `net.Listen` no servidor) na porta `8080`.
- **Formato:** cada mensagem é serializada como JSON e finalizada com `\n` para delimitar pacotes.
- **Fluxo básico:**
  1. O cliente estabelece a conexão (`net.Dial("tcp", "localhost:8080")`).
  2. Logo após conectar, envia uma mensagem de *handshake*.
  3. Em seguida envia atualizações periódicas de CPU e aceita comandos interativos do usuário.
  4. O servidor lê linha a linha (`bufio.Reader.ReadBytes('\n')`), decodifica JSON e trata conforme o tipo da mensagem.

> O uso de `\n` como terminador simplifica o framing das mensagens TCP, garantindo que o servidor saiba onde uma mensagem termina mesmo que a camada de transporte entregue os bytes em blocos diferentes.

---

## Estrutura do Protocolo (`libs/protocol`)

Todas as mensagens seguem a mesma estrutura:

```json
{
  "type": "<tipo-da-mensagem>",
  "data": { ... }
}
```

### Tipos implementados

| Tipo (`Message.Type`) | Payload (`Message.Data`) | Descrição |
| --- | --- | --- |
| `handshake` | `HandshakeData` (`client_id`, `version`) | Identifica o cliente assim que conecta. |
| `cpu_usage` | `CpuUsageData` (`usage`, `cores_usage`) | Porcentagem total e por núcleo da CPU. |
| `memory_usage` | `MemoryUsageData` (`total`, `used`, `used_percent`) | Snapshot da memória RAM. |
| `disk_usage` | `DiskUsageData` (`total`, `used`, `free`, `used_percent`) | Uso do disco no volume raiz. |
| `general_data` | `GeneralData` (`model_name`, `cores`, `mhz`) | Metadados básicos da CPU. |

No servidor, o pacote `libs/utils` fornece `ParseData`, que transforma o `interface{}` recebido no struct correspondente usando (re)serialização JSON.

---

## Handshake inicial

- **Cliente:** a função `sendHandshake(conn net.Conn)` prepara a mensagem:

  ```go
  protocol.Message{
    Type: "handshake",
    Data: protocol.HandshakeData{
      ClientID: "client123",
      Version:  "1.0.0",
    },
  }
  ```

- **Servidor:** ao receber, a `handleConnection` detecta `msg.Type == "handshake"`, converte os dados para `HandshakeData` e registra no console:

  ```
  🤝 Handshake received from 127.0.0.1:XXXXX: ClientID=client123, Version=1.0.0
  ```

Esse passo estabelece identificação lógica do cliente antes de qualquer telemetria.

---

## Ticker de CPU e Comando `/interval`

- O cliente inicia um *ticker* (`time.NewTicker`) com intervalo padrão de 5 segundos para `sendCpuUsage`.
- O usuário pode alterar o período digitando `/interval <ms>` no terminal do cliente.
- Internamente, o canal `intervalUpdates` atualiza o ticker com a nova duração, permitindo ajustar o ritmo de envio em tempo de execução.

O payload enviado contém a média da CPU (`usage`) e uma lista com o consumo por núcleo (`cores_usage`).

---

## Outras métricas disponíveis

Embora apenas o envio de CPU esteja automático, existem funções prontas para enviar:

- `sendMemoryUsage`: usa `gopsutil/mem` para coletar estatísticas da RAM.
- `sendDiskUsage`: via `gopsutil/disk.Usage("/")`.
- `sendGeneralData`: usa `gopsutil/cpu.Info()` para recuperar modelo, núcleos e clock.

Essas funções seguem o mesmo padrão: coletam os dados, constroem `protocol.Message`, serializam e escrevem no socket terminando com `\n`.

---

## Executando o Projeto

1. **Iniciar o servidor:**
   ```bash
   go run services/server/main.go --port 8080
   ```
   - `--port` (opcional, padrão `8080`): porta TCP em que o servidor ficará escutando.
   Saída esperada: `🚀 TCP server listening on :8080...`

2. **Rodar o cliente em outro terminal:**
   ```bash
   go run services/client/main.go --host localhost --port 8080 --id client123
   ```
   - `--host` (padrão `localhost`): endereço/IP do servidor.
   - `--port` (padrão `8080`): porta TCP do servidor.
   - `--id` (padrão `client`): identificador enviado no handshake.
   O cliente conecta, envia o handshake e passa a aceitar entradas interativas.

3. **Abrir o monitor (opcional):**
   ```bash
   go run services/monitor/main.go --host localhost --port 8080
   ```
   - `--host` (padrão `localhost`): endereço/IP do servidor.
   - `--port` (padrão `8080`): porta TCP do servidor.
   A interface mostra os clientes conectados; use ↑/↓ para navegar, `r` para solicitar snapshot, `q`/Esc para sair.

4. **Interagir:**
   - Observe no servidor os logs de handshake e demais mensagens.
   - No cliente, use `/interval 1000` para alterar o envio de CPU para 1 segundo.
   - Digite qualquer outro texto para enviar como linha crua (será ecoado pelo servidor apenas se houver tratamento adicional).

---

## Extensões sugeridas

- Persistir dados recebidos no servidor (banco ou arquivo) para histórico.
- Criar novos tipos no protocolo, como alertas (`alert`) ou métricas de rede.
- Implementar resposta do servidor para cada mensagem reconhecida, fechando o ciclo de confirmação.
- Validar versão do cliente durante o handshake para garantir compatibilidade.

Essas evoluções aproveitam a base do protocolo e o canal TCP já estabelecido, mantendo o formato JSON e o delimitador `\n` para garantir mensagens legíveis e fáceis de depurar.
