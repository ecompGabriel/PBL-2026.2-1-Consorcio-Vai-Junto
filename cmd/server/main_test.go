package main

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"vaijunto/internal/auth"
	"vaijunto/internal/config"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/domain"
	"vaijunto/internal/protocol"
	"vaijunto/internal/store"
)

func novoContextoServidorTeste(catalogo *store.CatalogoCaronas, usuarios *auth.GerenciadorUsuarios) *contextoServidor {
	cfg := &config.Config{
		EmpresaID: "empresa-1",
		Parceiros: make(map[string]config.Parceiro),
	}
	coordenador := consorcio.NovoCoordenadorTransacao("empresa-1", catalogo, cfg.Parceiros, nil)
	return &contextoServidor{
		config:      cfg,
		catalogo:    catalogo,
		usuarios:    usuarios,
		coordenador: coordenador,
	}
}

func TestTratarConexaoRejeitaJSONInvalidoSemCair(t *testing.T) {
	// net.Pipe cria duas pontas de net.Conn em memória. Ele permite testar o
	// protocolo sem abrir uma porta TCP real durante a suíte automatizada.
	servidor, cliente := net.Pipe()
	finalizou := make(chan struct{})

	go func() {
		tratarConexao(
			servidor,
			novoContextoServidorTeste(
				store.NovoCatalogoCaronas(),
				auth.NovoGerenciadorUsuarios(),
			),
		)
		close(finalizou)
	}()

	_, err := cliente.Write([]byte("{json inválido}\n"))
	if err != nil {
		t.Fatalf("não esperava erro ao enviar mensagem inválida: %v", err)
	}

	var resposta protocol.Resposta
	err = json.NewDecoder(cliente).Decode(&resposta)
	if err != nil {
		t.Fatalf("não esperava erro ao ler resposta: %v", err)
	}

	if resposta.Sucesso || resposta.Codigo != "mensagem_invalida" {
		t.Errorf(
			"resposta = sucesso %v, código %s; esperava mensagem_invalida",
			resposta.Sucesso,
			resposta.Codigo,
		)
	}

	cliente.Close()
	aguardarFimDaConexao(t, finalizou)
}

func TestTratarConexaoSuportaDesconexaoDuranteMensagem(t *testing.T) {
	// Simula um cliente que cai no meio de um JSON. O servidor deve encerrar só
	// esta goroutine, sem travar nem corromper seu estado compartilhado.
	servidor, cliente := net.Pipe()
	finalizou := make(chan struct{})

	go func() {
		tratarConexao(
			servidor,
			novoContextoServidorTeste(
				store.NovoCatalogoCaronas(),
				auth.NovoGerenciadorUsuarios(),
			),
		)
		close(finalizou)
	}()

	_, err := cliente.Write([]byte(`{"versao":"1.0","id":"req-1"`))
	if err != nil {
		t.Fatalf("não esperava erro ao enviar mensagem parcial: %v", err)
	}

	cliente.Close()
	aguardarFimDaConexao(t, finalizou)
}

func TestVariosClientesTCPNaoCausamOverbooking(t *testing.T) {
	// Cenário de carga: vinte passageiros disputarão, no mesmo instante, as
	// cinco vagas de um único trecho já publicado no catálogo.
	catalogo := store.NovoCatalogoCaronas()
	usuarios := auth.NovoGerenciadorUsuarios()
	ctxSrv := novoContextoServidorTeste(catalogo, usuarios)

	carona, err := domain.NovaCarona(
		"empresa-1",
		"carona-carga",
		"motorista-carga",
		time.Now().Add(time.Hour),
		[]string{"Feira", "Salvador"},
		5,
		[]int64{2000},
		[]int{60},
	)
	if err != nil {
		t.Fatalf("não esperava erro ao criar carona: %v", err)
	}

	err = catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	const quantidadeClientes = 20
	prontos := make(chan struct{}, quantidadeClientes)
	inicio := make(chan struct{})
	resultados := make(chan resultadoCliente, quantidadeClientes)
	var grupo sync.WaitGroup

	for i := 0; i < quantidadeClientes; i++ {
		usuarioID := fmt.Sprintf("passageiro-tcp-%d", i)
		err = usuarios.Registrar(usuarioID, "senha123", protocol.PerfilPassageiro)
		if err != nil {
			t.Fatalf("não esperava erro ao registrar usuário: %v", err)
		}

		grupo.Add(1)
		go executarClienteConcorrente(i, usuarioID, ctxSrv, prontos, inicio, resultados, &grupo)
	}

	// Só libera a disputa depois que todos os clientes terminaram o login e
	// sinalizaram que estão prontos. Isso torna a concorrência reproduzível.
	for i := 0; i < quantidadeClientes; i++ {
		<-prontos
	}

	inicioMedicao := time.Now()
	close(inicio)
	grupo.Wait()
	duracao := time.Since(inicioMedicao)
	close(resultados)

	sucessos := 0
	for resultado := range resultados {
		if resultado.err != nil {
			t.Errorf("cliente falhou na comunicação: %v", resultado.err)
			continue
		}
		if resultado.sucesso {
			sucessos++
		}
	}

	// A capacidade é cinco: mais sucessos indicariam overbooking; menos poderiam
	// indicar perda indevida de vagas ou falha de sincronização.
	if sucessos != 5 {
		t.Errorf("reservas confirmadas = %d; esperava 5", sucessos)
	}

	trechos := carona.Trechos()
	if trechos[0].AssentosDisponiveis != 0 {
		t.Errorf("assentos disponíveis = %d; esperava 0", trechos[0].AssentosDisponiveis)
	}

	t.Logf(
		"carga TCP: %d clientes, %d sucessos, %d falhas, duração %s",
		quantidadeClientes,
		sucessos,
		quantidadeClientes-sucessos,
		duracao,
	)
}

type resultadoCliente struct {
	sucesso bool
	err     error
}

func executarClienteConcorrente(
	numero int,
	usuarioID string,
	ctxSrv *contextoServidor,
	prontos chan<- struct{},
	inicio <-chan struct{},
	resultados chan<- resultadoCliente,
	grupo *sync.WaitGroup,
) {
	defer grupo.Done()

	// Cada cliente ganha uma conexão e uma sessão próprias, mas todos apontam
	// para o mesmo catálogo, que é o estado concorrente sob teste.
	servidor, cliente := net.Pipe()
	finalizou := make(chan struct{})
	go func() {
		tratarConexao(servidor, ctxSrv)
		close(finalizou)
	}()

	encoder := json.NewEncoder(cliente)
	decoder := json.NewDecoder(cliente)

	login := protocol.IniciarSessao{UsuarioID: usuarioID, Senha: "senha123"}
	resposta, err := enviarRequisicaoTeste(encoder, decoder, fmt.Sprintf("login-%d", numero), "iniciar_sessao", login)
	prontos <- struct{}{}
	if err != nil || !resposta.Sucesso {
		if err == nil {
			err = fmt.Errorf("login recusado: %s", resposta.Mensagem)
		}
		resultados <- resultadoCliente{err: err}
		cliente.Close()
		<-finalizou
		return
	}

	<-inicio

	confirmacao := protocol.ConfirmarReserva{
		IDReserva:          fmt.Sprintf("reserva-tcp-%d", numero),
		QuantidadeAssentos: 1,
		Trechos: []protocol.ReferenciaTrecho{
			{CaronaID: "carona-carga", Ordem: 0},
		},
	}

	resposta, err = enviarRequisicaoTeste(
		encoder,
		decoder,
		fmt.Sprintf("reserva-%d", numero),
		"confirmar_reserva",
		confirmacao,
	)
	resultados <- resultadoCliente{sucesso: resposta.Sucesso, err: err}

	cliente.Close()
	<-finalizou
}

func enviarRequisicaoTeste(
	encoder *json.Encoder,
	decoder *json.Decoder,
	id string,
	operacao string,
	dados any,
) (protocol.Resposta, error) {
	dadosJSON, err := json.Marshal(dados)
	if err != nil {
		return protocol.Resposta{}, err
	}

	requisicao := protocol.Requisicao{
		Versao:   protocol.VersaoAtual,
		ID:       id,
		Operacao: operacao,
		Dados:    dadosJSON,
	}

	err = encoder.Encode(requisicao)
	if err != nil {
		return protocol.Resposta{}, err
	}

	var resposta protocol.Resposta
	err = decoder.Decode(&resposta)
	return resposta, err
}

func aguardarFimDaConexao(t *testing.T, finalizou <-chan struct{}) {
	t.Helper()

	select {
	case <-finalizou:
	case <-time.After(time.Second):
		t.Fatal("o tratador da conexão não terminou após a desconexão")
	}
}
