# VaiJunto

Projeto individual de TEC502 (UEFS): sistema de caronas compartilhadas em Go,
com servidor central concorrente, clientes de terminal e comunicação por sockets
TCP/IP usando JSON.

## Funcionalidades

- cadastro e autenticação em memória de motoristas e passageiros;
- publicação de caronas com rota, horário, duração, capacidade e preço por trecho;
- busca de itinerários, inclusive combinando caronas diferentes e respeitando horários;
- comparação de cidades sem diferenciar maiúsculas e minúsculas;
- confirmação atômica de todos os trechos de uma reserva;
- consulta e cancelamento de reservas pelo passageiro;
- consulta de passageiros confirmados por trecho e cancelamento de carona pelo motorista;
- atendimento simultâneo por goroutine e proteção do estado com mutex;
- testes de domínio, protocolo, falhas de socket, atomicidade e overbooking;
- execução local ou em contêineres Docker.

## Arquitetura

```text
driver-client ──┐
                ├── TCP + JSON por linha ── server ── estado em memória
passenger-client┘                            ├── usuários
                                             ├── caronas/trechos
                                             └── reservas
```

- `internal/domain`: entidades e regras locais.
- `internal/search`: busca DFS no grafo de trechos.
- `internal/store`: estado compartilhado, mutex e transações de reserva.
- `internal/auth`: cadastro e validação de senhas com hash e salt.
- `internal/protocol`: representação intermediária JSON.
- `internal/clienttcp`: transporte comum usado pelos clientes.
- `cmd/server`: servidor central.
- `cmd/driver-client`: interface do motorista.
- `cmd/passenger-client`: interface do passageiro.

## Execução local

Requer Go 1.26 ou compatível.

Em um terminal:

```bash
go run ./cmd/server
```

Em outros terminais:

```bash
go run ./cmd/driver-client
go run ./cmd/passenger-client
```

Na primeira utilização de cada ID, escolha `Cadastrar e entrar`. Enquanto o
servidor estiver em execução, os cadastros, caronas e reservas permanecem em
memória. Reiniciar o servidor apaga esse estado.

Para conectar a outro computador:

```bash
VAIJUNTO_SERVER=IP_DO_SERVIDOR:3080 go run ./cmd/passenger-client
```

O servidor escuta em `:3080` por padrão. A variável `VAIJUNTO_LISTEN` permite
alterar o endereço de escuta.

## Testes

```bash
go test ./...
go vet ./...
go test -race ./...
```

O teste concorrente dispara 20 passageiros ao mesmo tempo para disputar cinco
vagas. Ele exige exatamente cinco sucessos, disponibilidade final zero e cinco
reservas armazenadas.

Para imprimir as métricas medidas na execução:

```bash
go test -run TestCatalogoImpedeOverbookingComPassageirosConcorrentes -v ./internal/store
go test -run TestVariosClientesTCPNaoCausamOverbooking -v ./cmd/server
```

## Docker

Construir e iniciar o servidor:

```bash
docker compose up --build server
```

O estágio de construção executa `go test ./...` dentro do contêiner antes de
gerar os binários.

Em outro terminal, executar um cliente interativo na mesma rede do Compose:

```bash
docker compose run --rm driver-client
docker compose run --rm passenger-client
```

Para executar um cliente em outra máquina, publique a porta `3080` no
computador servidor e informe o IP dele:

```bash
docker build -t vaijunto .
docker run --rm -it \
  -e VAIJUNTO_SERVER=IP_DO_SERVIDOR:3080 \
  vaijunto /app/driver-client
```

```bash
docker build -t vaijunto .
docker run --rm -it \
  -e VAIJUNTO_SERVER=IP_DO_SERVIDOR:3080 \
  vaijunto /app/passenger-client
```

As redes internas do Docker não atravessam computadores. Substitua
`IP_DO_SERVIDOR` pelo IP real da máquina que executa o servidor, obtido com
`hostname -I`, e use a porta publicada `3080`. O firewall da máquina servidora
deve permitir TCP nessa porta.

## Regras de concorrência

- a busca é apenas uma fotografia e não garante disponibilidade futura;
- a primeira saída ocorre na data escolhida ou depois; ao trocar de carona, o
  passageiro precisa de pelo menos 30 minutos para a conexão;
- a confirmação revalida todos os trechos com o mutex travado;
- todos os trechos são reservados ou nenhum é;
- o cancelamento pertence ao passageiro que criou a reserva;
- o motorista só cancela as próprias caronas; reservas que as usam são
  canceladas por inteiro, devolvendo todos os seus trechos;
- cancelar duas vezes não devolve assentos duas vezes;
- a disponibilidade sempre permanece entre zero e a capacidade.

## Limitações conhecidas

- os dados não são persistidos em banco ou arquivo;
- a senha é armazenada apenas como hash com salt, mas trafega em TCP sem TLS;
- a sessão dura enquanto a conexão TCP permanecer aberta ou até o timeout;
- as durações são estimadas pelo motorista e não consideram atrasos reais,
  trânsito ou cancelamentos de viagem;
- a busca limita ciclos por cidade visitada e retorna no máximo 20 resultados
  com até oito trechos cada.

Veja a especificação do protocolo em `docs/protocolo.md`.
