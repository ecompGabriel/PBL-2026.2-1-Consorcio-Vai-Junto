# VaiJunto — Problema 2: Consórcio de Caronas Compartilhadas

Projeto da disciplina **TEC502 (Concorrência e Conectividade)** — Universidade Estadual de Feira de Santana (UEFS).

Evolução da arquitetura centralizada do Problema 1 para um **consórcio descentralizado entre empresas independentes**, sem coordenador fixo, sem banco de dados compartilhado e sem bibliotecas externas de consenso distribuído (sem ZooKeeper, Redis, etcd ou XA).

---

## 1. Visão Geral da Arquitetura

O sistema opera de forma descentralizada. Cada empresa parceira opera o seu próprio servidor autônomo e mantém soberania exclusiva sobre seus dados, atendendo seus clientes locais via sockets TCP e cooperando com as demais empresas por meio de uma API REST HTTP.

```text
Motorista (A) ──┐                                    ┌── Servidor Empresa B (TCP :3080 / REST :3090)
                ├── TCP + JSON ── Servidor Empresa A ┼── API REST HTTP (Busca + 2PC)
Passageiro (A) ─┘                 (TCP :3080 / :3090)└── Servidor Empresa C (TCP :3080 / REST :3090)
                                         │
                                  Estado Persistente
                              (Usuários, Caronas, Reservas)
```

- **Dual-Stack por Empresa:**
  - **TCP (`:3080`):** Conexão com clientes locais de terminal (motoristas e passageiros) via sockets com mensagens JSON delimitadas por quebra de linha (`\n`).
  - **REST HTTP (`:3090`):** Comunicação entre os servidores do consórcio (busca distribuída de trechos em tempo real, votação e confirmação de transações).
- **Two-Phase Commit (2PC):** Coordenação distribuída para reserva de itinerários multiempresa. A empresa onde o passageiro está conectado atua como coordenadora temporária daquela transação.
- **Exclusão Mútua e Precedência:** Cada empresa é a única autoridade sobre os assentos das suas próprias caronas, serializando as requisições localmente por trecho (`sync.Mutex` por trecho + `sync.RWMutex` do catálogo), sem depender de relógios físicos sincronizados.
- **Prevenção de Deadlock Distribuído:** Ordenação alfabética canônica das empresas envolvidas antes de iniciar a aquisição de recursos provisórios (Fase 1 do 2PC) e ordenação determinística de trechos locais.
- **Tolerância a Falhas e Leases (TTL):** Reservas em preparação possuem tempo de vida útil (*lease*) e todas as chamadas REST possuem *timeout*. Caso o coordenador caia antes de enviar o Commit ou Abort, os assentos provisórios são liberados automaticamente pelos participantes após a expiração.
- **Propagação de Cancelamentos:** Cancelamentos de caronas ou de reservas são propagados via chamadas REST para liberar os assentos correspondentes nas demais empresas parceiras.
- **Persistência e Recuperação:** Armazenamento em snapshots JSON em disco (`dados/<empresa>.json` e `dados/<empresa>_usuarios.json`), com volumes persistentes no Docker.

---

## 2. Funcionalidades e Regras de Concorrência

- **Cadastro e Autenticação Persistente:** Registro de motoristas e passageiros com senhas protegidas por *salt* aleatório e hash SHA-256.
- **Oferta e Busca Federada de Caronas:** Publicação de caronas multi-trecho e busca em profundidade (DFS com *backtracking*) combinando trechos locais e remotos do consórcio, respeitando o tempo mínimo de 30 minutos para transbordo ao trocar de carona/empresa.
- **Reserva Atômica Multiempresa:** Todos os trechos de um itinerário (mesmo pertencendo a empresas distintas) são reservados juntos ou nenhum é reservado.
- **Cancelamento Consistente:** O cancelamento pelo passageiro ou o cancelamento da carona pelo motorista devolve os assentos em todas as empresas envolvidas na viagem, impedindo devolução duplicada (*idempotência*).
- **Degradação Graciosa:** Caso o servidor de uma empresa parceira esteja indisponível durante a busca, os demais servidores continuam operando e compondo itinerários com as empresas ativas.

---

## 3. Estrutura de Diretórios

- `cmd/server`: Ponto de entrada do servidor dual-stack (TCP e REST).
- `cmd/driver-client`: Cliente interativo de terminal para o motorista.
- `cmd/passenger-client`: Cliente interativo de terminal para o passageiro.
- `cmd/simulator`: Simulador autônomo de carga e concorrência (sem interface gráfica).
- `internal/consorcio`: Coordenador do 2PC, servidor REST e cliente HTTP do consórcio.
- `internal/store`: Catálogo de caronas e reservas com exclusão mútua granular (mutex por trecho), controle de leases e persistência.
- `internal/search`: Algoritmo de busca em profundidade (DFS) com detecção de conexões de itinerários e tempo mínimo de transbordo (30 minutos).
- `internal/auth`: Cadastro, autenticação e persistência de credenciais com salt e hash SHA-256.
- `internal/domain`: Entidades centrais do domínio (`Carona`, `Trecho`, `Reserva`, `Itinerario`).
- `internal/protocol`: Formato das mensagens JSON trocadas via TCP.
- `internal/config`: Leitura e validação de configurações e variáveis de ambiente.
- `docs/api_rest.md`: Especificação técnica dos endpoints REST do consórcio e diagrama de sequência do 2PC.
- `docs/protocolo.md`: Especificação das mensagens TCP com os clientes locais.

---

## 4. Como Executar em 1 Máquina (Ambiente Local com Docker Compose)

Na emulação local com as 3 empresas rodando na mesma máquina (`127.0.0.1` / `localhost`), o `docker-compose.yml` mapeia as seguintes portas:

| Empresa | Porta TCP (Clientes) | Porta REST (Consórcio) | Volume Persistente |
| :--- | :--- | :--- | :--- |
| **empresa-a** | `3081:3080` | `3091:3090` | `dados-empresa-a` |
| **empresa-b** | `3082:3080` | `3092:3090` | `dados-empresa-b` |
| **empresa-c** | `3083:3080` | `3093:3090` | `dados-empresa-c` |

### 4.1 Subir os servidores do consórcio:

Para subir as **3 empresas juntas** no Docker Compose:

```bash
docker compose up -d --build
```

*(Opcional) Para subir **apenas 1 servidor isolado** localmente:*
```bash
# Via Docker Compose (ex.: apenas a empresa-a):
docker compose up -d --build empresa-a

# Ou diretamente com Go local (sobe nas portas padrão TCP :3080 e REST :3090):
go run ./cmd/server
```

Verificar se os contêineres estão ativos e respondendo via REST:

```bash
docker ps
curl -s http://localhost:3091/api/v1/trechos
curl -s http://localhost:3092/api/v1/trechos
curl -s http://localhost:3093/api/v1/trechos
```

### 4.2 Conectar Motoristas e Passageiros (em novos terminais):

Via Docker (conectando diretamente na rede de cada contêiner):
```bash
# Motorista ou Passageiro na Empresa A:
docker run -it --rm --network container:empresa-a -v $(pwd):/app -w /app golang:1.26 go run ./cmd/driver-client
docker run -it --rm --network container:empresa-a -v $(pwd):/app -w /app golang:1.26 go run ./cmd/passenger-client

# Motorista ou Passageiro na Empresa B:
docker run -it --rm --network container:empresa-b -v $(pwd):/app -w /app golang:1.26 go run ./cmd/driver-client
docker run -it --rm --network container:empresa-b -v $(pwd):/app -w /app golang:1.26 go run ./cmd/passenger-client

# Motorista ou Passageiro na Empresa C:
docker run -it --rm --network container:empresa-c -v $(pwd):/app -w /app golang:1.26 go run ./cmd/driver-client
docker run -it --rm --network container:empresa-c -v $(pwd):/app -w /app golang:1.26 go run ./cmd/passenger-client
```

Ou diretamente com Go instalado na máquina local (`127.0.0.1` aponta para a própria máquina):
```bash
# Motorista na Empresa A (porta 3081):
VAIJUNTO_SERVER="127.0.0.1:3081" go run ./cmd/driver-client

# Motorista na Empresa B (porta 3082):
VAIJUNTO_SERVER="127.0.0.1:3082" go run ./cmd/driver-client

# Passageiro na Empresa C (porta 3083):
VAIJUNTO_SERVER="127.0.0.1:3083" go run ./cmd/passenger-client
```

---

## 5. Como Executar em 3 Computadores Distintos (1 Empresa por Máquina)

Para executar cada empresa em um computador físico independente na rede local do laboratório (substituindo `IP1`, `IP2` e `IP3` pelos IPs reais de cada máquina, obtidos com `hostname -I`):

### 5.1 Construir a imagem em cada computador:
```bash
docker build -t vaijunto-node .
```

### 5.2 No PC 1 (`IP1` — Empresa A):
```bash
docker rm -f empresa-a
docker run -d --name empresa-a \
  -e VAIJUNTO_EMPRESA_ID=empresa-a \
  -e VAIJUNTO_TCP_LISTEN=:3080 \
  -e VAIJUNTO_REST_LISTEN=:3090 \
  -e VAIJUNTO_PARCEIROS="empresa-b=http://IP2:3090,empresa-c=http://IP3:3090" \
  -p 3080:3080 -p 3090:3090 \
  -v dados-empresa-a:/app/dados \
  vaijunto-node

# Conectar Motorista e Passageiro na Empresa A:
docker run -it --rm --network container:empresa-a -v $(pwd):/app -w /app golang:1.26 go run ./cmd/driver-client
docker run -it --rm --network container:empresa-a -v $(pwd):/app -w /app golang:1.26 go run ./cmd/passenger-client
```

### 5.3 No PC 2 (`IP2` — Empresa B):
```bash
docker rm -f empresa-b
docker run -d --name empresa-b \
  -e VAIJUNTO_EMPRESA_ID=empresa-b \
  -e VAIJUNTO_TCP_LISTEN=:3080 \
  -e VAIJUNTO_REST_LISTEN=:3090 \
  -e VAIJUNTO_PARCEIROS="empresa-a=http://IP1:3090,empresa-c=http://IP3:3090" \
  -p 3080:3080 -p 3090:3090 \
  -v dados-empresa-b:/app/dados \
  vaijunto-node

# Conectar Motorista e Passageiro na Empresa B:
docker run -it --rm --network container:empresa-b -v $(pwd):/app -w /app golang:1.26 go run ./cmd/driver-client
docker run -it --rm --network container:empresa-b -v $(pwd):/app -w /app golang:1.26 go run ./cmd/passenger-client
```

### 5.4 No PC 3 (`IP3` — Empresa C):
```bash
docker rm -f empresa-c
docker run -d --name empresa-c \
  -e VAIJUNTO_EMPRESA_ID=empresa-c \
  -e VAIJUNTO_TCP_LISTEN=:3080 \
  -e VAIJUNTO_REST_LISTEN=:3090 \
  -e VAIJUNTO_PARCEIROS="empresa-a=http://IP1:3090,empresa-b=http://IP2:3090" \
  -p 3080:3080 -p 3090:3090 \
  -v dados-empresa-c:/app/dados \
  vaijunto-node

# Conectar Motorista e Passageiro na Empresa C:
docker run -it --rm --network container:empresa-c -v $(pwd):/app -w /app golang:1.26 go run ./cmd/driver-client
docker run -it --rm --network container:empresa-c -v $(pwd):/app -w /app golang:1.26 go run ./cmd/passenger-client
```

---

## 6. Simulador de Carga e Testes Automatizados

### 6.1 Simulador Autônomo de Carga Concorrente
O simulador instancia as 3 empresas com servidores HTTP reais e dispara múltiplos passageiros simultâneos através de barreira de sincronização:

```bash
# Via Docker:
docker run -it --rm --network host -v $(pwd):/app -w /app golang:1.26 go run ./cmd/simulator

# Ou com Go local:
go run ./cmd/simulator
go run ./cmd/simulator -passageiros 50
```

### 6.2 Suíte de Testes e Detector de Condições de Corrida (`-race`)

```bash
# Via Docker:
docker run --rm -v $(pwd):/app -w /app golang:1.26 go test -race -v ./...
docker run --rm -v $(pwd):/app -w /app golang:1.26 go test -v -count=1 -run TestSimuladorComInjecaoDeFalhas ./cmd/simulator

# Ou com Go local:
go test -v ./...
go test -race ./...
go vet ./...
```

---

## 7. Monitoramento e Injeção de Falhas no Docker

Acompanhar os logs em tempo real de cada servidor:
```bash
docker logs -f empresa-a
docker logs -f empresa-b
docker logs -f empresa-c
```

Simular falhas de servidor ou de rede e recuperar mantendo os volumes persistidos:
```bash
# Parada do contêiner (SIGTERM) ou queda abrupta (SIGKILL):
docker stop empresa-c
docker kill empresa-c

# Congelar e descongelar processo (simular timeout / partição):
docker pause empresa-c
docker unpause empresa-c

# Religar o servidor recuperando o estado salvo em disco:
docker start empresa-c
```
