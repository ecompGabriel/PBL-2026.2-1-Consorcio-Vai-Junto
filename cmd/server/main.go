package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"vaijunto/internal/auth"
	"vaijunto/internal/config"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/domain"
	"vaijunto/internal/protocol"
	"vaijunto/internal/search"
	"vaijunto/internal/store"
)

const (
	tamanhoMaximoMensagem   = 64 * 1024
	tempoMaximoSemAtividade = 45 * time.Minute
	tempoMaximoEscrita      = 10 * time.Second
)

type contextoServidor struct {
	config      *config.Config
	catalogo    *store.CatalogoCaronas
	usuarios    *auth.GerenciadorUsuarios
	clienteHTTP *consorcio.ClienteConsorcio
	coordenador *consorcio.CoordenadorTransacao
}

type sessaoConexao struct {
	UsuarioID string
	Perfil    string
}

func main() {
	cfg, err := config.Carregar()
	if err != nil {
		// Se não houver VAIJUNTO_EMPRESA_ID definida, usa valores de fallback padrão para testes locais
		cfg = &config.Config{
			EmpresaID:  "empresa-1",
			TCPListen:  config.PortaTCPPadrao,
			RESTListen: config.PortaRESTPadrao,
			Parceiros:  make(map[string]config.Parceiro),
		}
	}

	caminhoSnapshot := fmt.Sprintf("dados/%s.json", cfg.EmpresaID)
	catalogo := store.NovoCatalogoComPersistencia(caminhoSnapshot)
	usuarios := auth.NovoGerenciadorUsuarios()

	servidorREST := consorcio.NovoServidor(cfg.EmpresaID, catalogo)
	clienteREST := consorcio.NovoCliente(cfg.Parceiros, nil)
	coordenador := consorcio.NovoCoordenadorTransacao(cfg.EmpresaID, catalogo, cfg.Parceiros, nil)

	ctxServidor := &contextoServidor{
		config:      cfg,
		catalogo:    catalogo,
		usuarios:    usuarios,
		clienteHTTP: clienteREST,
		coordenador: coordenador,
	}

	// 1. Inicia o Servidor REST para comunicação interempresas em segundo plano
	go func() {
		log.Printf("[%s] API REST do consórcio ouvindo em %s", cfg.EmpresaID, cfg.RESTListen)
		if err := http.ListenAndServe(cfg.RESTListen, servidorREST.Handler()); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] erro no servidor REST: %v", cfg.EmpresaID, err)
		}
	}()

	// 2. Inicia o Servidor TCP para motoristas e passageiros locais
	listener, err := net.Listen("tcp", cfg.TCPListen)
	if err != nil {
		log.Fatalf("[%s] não foi possível iniciar o servidor TCP: %v", cfg.EmpresaID, err)
	}
	defer listener.Close()

	log.Printf("[%s] servidor TCP para clientes locais ouvindo em %s", cfg.EmpresaID, cfg.TCPListen)

	for {
		conexao, err := listener.Accept()
		if err != nil {
			log.Printf("[%s] erro ao aceitar conexão: %v", cfg.EmpresaID, err)
			continue
		}

		go tratarConexao(conexao, ctxServidor)
	}
}

func tratarConexao(
	conexao net.Conn,
	ctxSrv *contextoServidor,
) {
	defer conexao.Close()

	leitor := bufio.NewScanner(conexao)
	leitor.Buffer(make([]byte, 4096), tamanhoMaximoMensagem)
	sessao := sessaoConexao{}

	for {
		err := conexao.SetReadDeadline(time.Now().Add(tempoMaximoSemAtividade))
		if err != nil {
			return
		}

		if !leitor.Scan() {
			break
		}

		var requisicao protocol.Requisicao
		err = protocol.DecodificarEstrito(leitor.Bytes(), &requisicao)
		if err != nil {
			enviarResposta(conexao, respostaErro("", "mensagem_invalida", "requisição JSON inválida"))
			continue
		}

		resposta := tratarRequisicao(requisicao, &sessao, ctxSrv)
		if !enviarResposta(conexao, resposta) {
			return
		}
	}
}

func tratarRequisicao(
	requisicao protocol.Requisicao,
	sessao *sessaoConexao,
	ctxSrv *contextoServidor,
) protocol.Resposta {
	if requisicao.Versao != protocol.VersaoAtual {
		return respostaErro(
			requisicao.ID,
			"versao_incompativel",
			"versão do protocolo não suportada pelo servidor",
		)
	}

	if requisicao.Operacao == "registrar_usuario" {
		return tratarRegistrarUsuario(requisicao, sessao, ctxSrv.usuarios)
	}
	if requisicao.Operacao == "iniciar_sessao" {
		return tratarIniciarSessao(requisicao, sessao, ctxSrv.usuarios)
	}

	if sessao.UsuarioID == "" {
		return respostaErro(
			requisicao.ID,
			"sessao_nao_iniciada",
			"é necessário iniciar sessão para realizar esta operação",
		)
	}

	switch requisicao.Operacao {
	case "criar_carona":
		if sessao.Perfil != protocol.PerfilMotorista {
			return respostaErro(requisicao.ID, "perfil_nao_autorizado", "operação exclusiva do motorista")
		}
		return tratarCriarCarona(requisicao, ctxSrv.catalogo, ctxSrv.config.EmpresaID, sessao.UsuarioID)

	case "listar_caronas_motorista":
		if sessao.Perfil != protocol.PerfilMotorista {
			return respostaErro(requisicao.ID, "perfil_nao_autorizado", "operação exclusiva do motorista")
		}
		return tratarListarCaronasMotorista(requisicao, ctxSrv.catalogo, sessao.UsuarioID)

	case "cancelar_carona":
		if sessao.Perfil != protocol.PerfilMotorista {
			return respostaErro(requisicao.ID, "perfil_nao_autorizado", "operação exclusiva do motorista")
		}
		return tratarCancelarCarona(requisicao, ctxSrv.catalogo, sessao.UsuarioID)

	case "buscar_itinerarios":
		if sessao.Perfil != protocol.PerfilPassageiro {
			return respostaErro(requisicao.ID, "perfil_nao_autorizado", "operação exclusiva do passageiro")
		}
		return tratarBuscarItinerarios(requisicao, ctxSrv)

	case "confirmar_reserva":
		if sessao.Perfil != protocol.PerfilPassageiro {
			return respostaErro(requisicao.ID, "perfil_nao_autorizado", "operação exclusiva do passageiro")
		}
		return tratarConfirmarReserva(requisicao, ctxSrv, sessao.UsuarioID)

	case "consultar_reservas":
		if sessao.Perfil != protocol.PerfilPassageiro {
			return respostaErro(requisicao.ID, "perfil_nao_autorizado", "operação exclusiva do passageiro")
		}
		return tratarConsultarReservas(requisicao, ctxSrv.catalogo, sessao.UsuarioID)

	case "cancelar_reserva":
		if sessao.Perfil != protocol.PerfilPassageiro {
			return respostaErro(requisicao.ID, "perfil_nao_autorizado", "operação exclusiva do passageiro")
		}
		return tratarCancelarReserva(requisicao, ctxSrv.catalogo, sessao.UsuarioID)

	default:
		return respostaErro(requisicao.ID, "operacao_desconhecida", "operação desconhecida")
	}
}

func tratarRegistrarUsuario(
	requisicao protocol.Requisicao,
	sessao *sessaoConexao,
	usuarios *auth.GerenciadorUsuarios,
) protocol.Resposta {
	var dados protocol.RegistrarUsuario
	err := protocol.DecodificarEstrito(requisicao.Dados, &dados)
	if err != nil {
		return respostaErro(requisicao.ID, "dados_invalidos", "dados de cadastro inválidos")
	}

	err = usuarios.Registrar(dados.UsuarioID, dados.Senha, dados.Perfil)
	if err != nil {
		return respostaErro(requisicao.ID, "cadastro_recusado", err.Error())
	}

	sessao.UsuarioID = dados.UsuarioID
	sessao.Perfil = dados.Perfil

	return respostaSucesso(
		requisicao.ID,
		"usuário cadastrado com sucesso",
		protocol.SessaoIniciada{
			UsuarioID: dados.UsuarioID,
			Perfil:    dados.Perfil,
		},
	)
}

func tratarIniciarSessao(
	requisicao protocol.Requisicao,
	sessao *sessaoConexao,
	usuarios *auth.GerenciadorUsuarios,
) protocol.Resposta {
	var dados protocol.IniciarSessao
	err := protocol.DecodificarEstrito(requisicao.Dados, &dados)
	if err != nil {
		return respostaErro(requisicao.ID, "dados_invalidos", "credenciais inválidas")
	}

	perfil, err := usuarios.Autenticar(dados.UsuarioID, dados.Senha)
	if err != nil {
		return respostaErro(requisicao.ID, "autenticacao_falhou", err.Error())
	}

	sessao.UsuarioID = dados.UsuarioID
	sessao.Perfil = perfil

	return respostaSucesso(
		requisicao.ID,
		"sessão iniciada com sucesso",
		protocol.SessaoIniciada{
			UsuarioID: dados.UsuarioID,
			Perfil:    perfil,
		},
	)
}

func tratarCriarCarona(
	requisicao protocol.Requisicao,
	catalogo *store.CatalogoCaronas,
	empresaID string,
	motoristaID string,
) protocol.Resposta {
	var dados protocol.CriarCarona
	err := protocol.DecodificarEstrito(requisicao.Dados, &dados)
	if err != nil {
		return respostaErro(requisicao.ID, "dados_invalidos", "dados da carona inválidos")
	}

	horarioSaida, err := time.Parse(time.RFC3339, dados.HorarioSaida)
	if err != nil {
		return respostaErro(requisicao.ID, "horario_invalido", "horário de saída inválido")
	}

	carona, err := domain.NovaCarona(
		empresaID,
		dados.ID,
		motoristaID,
		horarioSaida,
		dados.Rota,
		dados.Capacidade,
		dados.PrecosCentavos,
		dados.DuracoesMinutos,
	)
	if err != nil {
		return respostaErro(requisicao.ID, "carona_invalida", err.Error())
	}

	err = catalogo.Adicionar(carona)
	if err != nil {
		return respostaErro(requisicao.ID, "carona_recusada", err.Error())
	}

	return respostaSucesso(requisicao.ID, "carona criada com sucesso", nil)
}

func tratarListarCaronasMotorista(
	requisicao protocol.Requisicao,
	catalogo *store.CatalogoCaronas,
	motoristaID string,
) protocol.Resposta {
	caronas := catalogo.ListarCaronasDoMotorista(motoristaID)

	resultados := make([]protocol.CaronaDoMotorista, 0, len(caronas))
	for _, carona := range caronas {
		trechos := make([]protocol.TrechoDoMotorista, 0, len(carona.Trechos))
		for _, trecho := range carona.Trechos {
			passageiros := make([]protocol.PassageiroConfirmado, 0, len(trecho.Passageiros))
			for _, p := range trecho.Passageiros {
				passageiros = append(passageiros, protocol.PassageiroConfirmado{
					PassageiroID:       p.PassageiroID,
					QuantidadeAssentos: p.QuantidadeAssentos,
				})
			}

			trechos = append(trechos, protocol.TrechoDoMotorista{
				Ordem:               trecho.Trecho.Ordem,
				Origem:              trecho.Trecho.Origem,
				Destino:             trecho.Trecho.Destino,
				Capacidade:          trecho.Trecho.Capacidade,
				AssentosDisponiveis: trecho.Trecho.AssentosDisponiveis,
				HorarioSaida:        trecho.Trecho.HorarioSaida.Format(time.RFC3339),
				HorarioChegada:      trecho.Trecho.HorarioChegada.Format(time.RFC3339),
				Passageiros:         passageiros,
			})
		}

		resultados = append(resultados, protocol.CaronaDoMotorista{
			ID:        carona.ID,
			Cancelada: carona.Cancelada,
			Trechos:   trechos,
		})
	}

	return respostaSucesso(requisicao.ID, "caronas do motorista consultadas", resultados)
}

func tratarCancelarCarona(
	requisicao protocol.Requisicao,
	catalogo *store.CatalogoCaronas,
	motoristaID string,
) protocol.Resposta {
	var dados protocol.CancelarCarona
	err := protocol.DecodificarEstrito(requisicao.Dados, &dados)
	if err != nil {
		return respostaErro(requisicao.ID, "dados_invalidos", "dados do cancelamento inválidos")
	}

	err = catalogo.CancelarCarona(dados.IDCarona, motoristaID)
	if err != nil {
		return respostaErro(requisicao.ID, "cancelamento_recusado", err.Error())
	}

	return respostaSucesso(requisicao.ID, "carona cancelada com sucesso", nil)
}

func tratarBuscarItinerarios(
	requisicao protocol.Requisicao,
	ctxSrv *contextoServidor,
) protocol.Resposta {
	var dados protocol.BuscarItinerarios
	err := protocol.DecodificarEstrito(requisicao.Dados, &dados)
	if err != nil {
		return respostaErro(requisicao.ID, "dados_invalidos", "dados da busca inválidos")
	}
	dataDesejada, err := time.Parse(time.RFC3339, dados.DataDesejada)
	if err != nil {
		return respostaErro(requisicao.ID, "data_invalida", "data desejada inválida")
	}

	// 1. Busca trechos locais
	trechosTotais := ctxSrv.catalogo.ListarTrechos()

	// 2. Busca trechos remotos das parceiras via REST
	trechosRemotos, _ := ctxSrv.clienteHTTP.BuscarTrechosEmTodasParceiras(context.Background())
	trechosTotais = append(trechosTotais, trechosRemotos...)

	// 3. Monta grafo temporário com trechos locais + remotos e executa DFS
	itinerarios, err := search.BuscarItinerarios(
		trechosTotais,
		dados.Origem,
		dados.Destino,
		dados.QuantidadeAssentos,
		dataDesejada,
	)
	if err != nil {
		return respostaErro(requisicao.ID, "busca_invalida", err.Error())
	}

	resultados := make([]protocol.ItinerarioEncontrado, 0, len(itinerarios))
	for _, itinerario := range itinerarios {
		trechos := make([]protocol.TrechoEncontrado, 0, len(itinerario.Trechos))
		for _, trecho := range itinerario.Trechos {
			trechos = append(trechos, protocol.TrechoEncontrado{
				EmpresaID:           trecho.EmpresaID,
				CaronaID:            trecho.CaronaID,
				Ordem:               trecho.Ordem,
				Origem:              trecho.Origem,
				Destino:             trecho.Destino,
				PrecoCentavos:       trecho.PrecoCentavos,
				AssentosDisponiveis: trecho.AssentosDisponiveis,
				HorarioSaida:        trecho.HorarioSaida.Format(time.RFC3339),
				HorarioChegada:      trecho.HorarioChegada.Format(time.RFC3339),
			})
		}

		resultados = append(resultados, protocol.ItinerarioEncontrado{
			Origem:             itinerario.Origem,
			Destino:            itinerario.Destino,
			Trechos:            trechos,
			PrecoTotalCentavos: itinerario.PrecoTotalCentavos,
		})
	}

	mensagem := "itinerários encontrados"
	if len(resultados) == 0 {
		mensagem = "nenhum itinerário encontrado"
	}

	return respostaSucesso(requisicao.ID, mensagem, resultados)
}

func tratarConfirmarReserva(
	requisicao protocol.Requisicao,
	ctxSrv *contextoServidor,
	passageiroID string,
) protocol.Resposta {
	var dados protocol.ConfirmarReserva
	err := protocol.DecodificarEstrito(requisicao.Dados, &dados)
	if err != nil {
		return respostaErro(requisicao.ID, "dados_invalidos", "dados da reserva inválidos")
	}

	referencias := make([]domain.ReferenciaTrecho, 0, len(dados.Trechos))
	for _, trecho := range dados.Trechos {
		empID := trecho.EmpresaID
		if empID == "" {
			empID = ctxSrv.config.EmpresaID
		}
		referencias = append(referencias, domain.ReferenciaTrecho{
			EmpresaID: empID,
			CaronaID:  trecho.CaronaID,
			Ordem:     trecho.Ordem,
		})
	}

	// Executa o Two-Phase Commit distribuído completo
	reserva, err := ctxSrv.coordenador.ReservarItinerario(
		context.Background(),
		dados.IDReserva,
		passageiroID,
		dados.QuantidadeAssentos,
		referencias,
	)
	if err != nil {
		return respostaErro(requisicao.ID, "reserva_nao_confirmada", err.Error())
	}

	trechosResposta := make([]protocol.ReferenciaTrecho, 0, len(reserva.Trechos()))
	for _, trecho := range reserva.Trechos() {
		trechosResposta = append(trechosResposta, protocol.ReferenciaTrecho{
			EmpresaID: trecho.EmpresaID,
			CaronaID:  trecho.CaronaID,
			Ordem:     trecho.Ordem,
		})
	}

	return respostaSucesso(
		requisicao.ID,
		"reserva confirmada com sucesso",
		protocol.Reserva{
			ID:                 reserva.ID,
			PassageiroID:       reserva.PassageiroID,
			QuantidadeAssentos: reserva.QuantidadeAssentos,
			Status:             string(reserva.Status),
			Trechos:            trechosResposta,
		},
	)
}

func tratarConsultarReservas(
	requisicao protocol.Requisicao,
	catalogo *store.CatalogoCaronas,
	passageiroID string,
) protocol.Resposta {
	reservas := catalogo.ListarReservasDoPassageiro(passageiroID)

	resultados := make([]protocol.Reserva, 0, len(reservas))
	for _, reserva := range reservas {
		trechos := make([]protocol.ReferenciaTrecho, 0, len(reserva.Trechos()))
		for _, trecho := range reserva.Trechos() {
			trechos = append(trechos, protocol.ReferenciaTrecho{
				EmpresaID: trecho.EmpresaID,
				CaronaID:  trecho.CaronaID,
				Ordem:     trecho.Ordem,
			})
		}

		resultados = append(resultados, protocol.Reserva{
			ID:                 reserva.ID,
			PassageiroID:       reserva.PassageiroID,
			QuantidadeAssentos: reserva.QuantidadeAssentos,
			Status:             string(reserva.Status),
			Trechos:            trechos,
		})
	}

	return respostaSucesso(requisicao.ID, "reservas do passageiro consultadas", resultados)
}

func tratarCancelarReserva(
	requisicao protocol.Requisicao,
	catalogo *store.CatalogoCaronas,
	passageiroID string,
) protocol.Resposta {
	var dados protocol.CancelarReserva
	err := protocol.DecodificarEstrito(requisicao.Dados, &dados)
	if err != nil {
		return respostaErro(requisicao.ID, "dados_invalidos", "dados do cancelamento inválidos")
	}

	err = catalogo.CancelarReserva(dados.IDReserva, passageiroID)
	if err != nil {
		return respostaErro(requisicao.ID, "cancelamento_recusado", err.Error())
	}

	return respostaSucesso(requisicao.ID, "reserva cancelada com sucesso", nil)
}

func enviarResposta(conexao net.Conn, resposta protocol.Resposta) bool {
	bytesResposta, err := json.Marshal(resposta)
	if err != nil {
		return false
	}

	bytesResposta = append(bytesResposta, '\n')
	err = conexao.SetWriteDeadline(time.Now().Add(tempoMaximoEscrita))
	if err != nil {
		return false
	}

	_, err = conexao.Write(bytesResposta)
	return err == nil
}

func respostaSucesso(id string, mensagem string, dados any) protocol.Resposta {
	var rawDados []byte
	var err error

	if dados != nil {
		rawDados, err = json.Marshal(dados)
		if err != nil {
			return respostaErro(id, "erro_interno", "não foi possível serializar a resposta")
		}
	}

	return protocol.Resposta{
		Versao:   protocol.VersaoAtual,
		ID:       id,
		Sucesso:  true,
		Codigo:   "ok",
		Mensagem: mensagem,
		Dados:    rawDados,
	}
}

func respostaErro(id string, codigo string, mensagem string) protocol.Resposta {
	return protocol.Resposta{
		Versao:   protocol.VersaoAtual,
		ID:       id,
		Sucesso:  false,
		Codigo:   codigo,
		Mensagem: mensagem,
	}
}
