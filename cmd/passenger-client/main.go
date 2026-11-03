package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"vaijunto/internal/clienttcp"
	"vaijunto/internal/protocol"
)

func main() {
	// O menu do passageiro reutiliza a conexão TCP aberta durante a sessão.
	leitor := bufio.NewReader(os.Stdin)

	cliente, err := clienttcp.Conectar()
	if err != nil {
		log.Fatalf("não foi possível conectar ao servidor: %v", err)
	}
	defer cliente.Fechar()

	if !autenticar(leitor, cliente, protocol.PerfilPassageiro) {
		return
	}

	for {
		fmt.Println("\nVaiJunto - Passageiro")
		fmt.Println("1 - Buscar itinerários")
		fmt.Println("2 - Consultar minhas reservas")
		fmt.Println("3 - Cancelar reserva")
		fmt.Println("0 - Sair")

		opcao := lerTexto(leitor, "Escolha: ")
		switch opcao {
		case "1":
			buscarEReservar(leitor, cliente)
		case "2":
			consultarReservas(cliente)
		case "3":
			cancelarReserva(leitor, cliente)
		case "0":
			fmt.Println("Até logo!")
			return
		default:
			fmt.Println("Opção inválida.")
		}
	}
}

func autenticar(leitor *bufio.Reader, cliente *clienttcp.Cliente, perfil string) bool {
	// Permite nova tentativa após credenciais inválidas sem encerrar o cliente.
	for {
		fmt.Println("\nAutenticação")
		fmt.Println("1 - Entrar")
		fmt.Println("2 - Cadastrar e entrar")
		fmt.Println("0 - Sair")

		opcao := lerTexto(leitor, "Escolha: ")
		if opcao == "0" {
			fmt.Println("Até logo!")
			return false
		}
		if opcao != "1" && opcao != "2" {
			fmt.Println("Opção inválida.")
			continue
		}

		usuarioID := lerTexto(leitor, "ID do usuário: ")
		senha := lerTexto(leitor, "Senha (mínimo de 4 caracteres): ")

		if opcao == "2" {
			resposta, err := cliente.RegistrarUsuario(usuarioID, senha, perfil)
			if err != nil {
				fmt.Println("Erro de comunicação ao cadastrar:", err)
				continue
			}
			if !resposta.Sucesso {
				fmt.Println("Cadastro recusado:", resposta.Mensagem)
				continue
			}
		}

		resposta, err := cliente.IniciarSessao(usuarioID, senha)
		if err != nil {
			fmt.Println("Erro de comunicação ao entrar:", err)
			continue
		}
		if !resposta.Sucesso {
			fmt.Println("Autenticação recusada:", resposta.Mensagem)
			continue
		}

		fmt.Println("Sessão iniciada com sucesso.")
		return true
	}
}

func buscarEReservar(leitor *bufio.Reader, cliente *clienttcp.Cliente) {
	// A busca mostra possibilidades, mas a vaga só é garantida na confirmação.
	origem := lerTexto(leitor, "Cidade de origem: ")
	destino := lerTexto(leitor, "Cidade de destino: ")
	dataDesejada := lerDataDesejada(leitor)
	quantidadeAssentos := lerInteiroPositivo(leitor, "Quantidade de assentos: ")

	dados := protocol.BuscarItinerarios{
		Origem:             origem,
		Destino:            destino,
		QuantidadeAssentos: quantidadeAssentos,
		DataDesejada:       dataDesejada,
	}

	var itinerarios []protocol.ItinerarioEncontrado
	resposta, err := cliente.Enviar("buscar_itinerarios", dados, &itinerarios)
	if err != nil {
		fmt.Println("Erro de comunicação:", err)
		return
	}
	if !resposta.Sucesso {
		fmt.Println("Busca recusada:", resposta.Mensagem)
		return
	}

	if len(itinerarios) == 0 {
		fmt.Println("Nenhum itinerário encontrado.")
		return
	}

	mostrarItinerarios(itinerarios)

	for {
		escolha := lerInteiroNaoNegativo(
			leitor,
			"Digite o número da opção para reservar ou 0 para voltar: ",
		)
		if escolha == 0 {
			return
		}
		if escolha > len(itinerarios) {
			fmt.Println("Opção inexistente.")
			continue
		}

		// A opção exibida começa em 1 para o usuário, enquanto slices de Go começam
		// em zero; por isso escolha-1 seleciona a posição correta.
		itinerario := itinerarios[escolha-1]
		// A reserva envia a identidade de cada trecho escolhido, não apenas o
		// itinerário exibido; assim o servidor consegue revalidar as vagas.
		referencias := make([]protocol.ReferenciaTrecho, 0, len(itinerario.Trechos))
		for _, trecho := range itinerario.Trechos {
			referencias = append(referencias, protocol.ReferenciaTrecho{
				EmpresaID: trecho.EmpresaID,
				CaronaID:  trecho.CaronaID,
				Ordem:     trecho.Ordem,
			})
		}

		// O ID técnico é criado automaticamente para evitar colisões entre
		// reservas simultâneas. O passageiro o consulta antes de cancelar.
		confirmar := protocol.ConfirmarReserva{
			IDReserva:          novoIDReserva(),
			QuantidadeAssentos: quantidadeAssentos,
			Trechos:            referencias,
		}

		var reserva protocol.Reserva
		resposta, err = cliente.Enviar("confirmar_reserva", confirmar, &reserva)
		if err != nil {
			fmt.Println("Erro de comunicação:", err)
			return
		}
		if !resposta.Sucesso {
			fmt.Println("Reserva recusada:", resposta.Mensagem)
			return
		}

		fmt.Printf("Reserva %s confirmada para %d assento(s).\n", reserva.ID, reserva.QuantidadeAssentos)
		return
	}
}

func consultarReservas(cliente *clienttcp.Cliente) {
	// A consulta é filtrada no servidor pelo usuário da sessão atual.
	var reservas []protocol.Reserva
	resposta, err := cliente.Enviar("consultar_reservas", struct{}{}, &reservas)
	if err != nil {
		fmt.Println("Erro de comunicação:", err)
		return
	}
	if !resposta.Sucesso {
		fmt.Println("Consulta recusada:", resposta.Mensagem)
		return
	}

	if len(reservas) == 0 {
		fmt.Println("Você ainda não possui reservas.")
		return
	}

	for _, reserva := range reservas {
		fmt.Printf(
			"\nReserva %s - %s - %d assento(s)\n",
			reserva.ID,
			reserva.Status,
			reserva.QuantidadeAssentos,
		)
		for _, trecho := range reserva.Trechos {
			fmt.Printf("- Carona %s, trecho %d\n", trecho.CaronaID, trecho.Ordem)
		}
	}
}

func cancelarReserva(leitor *bufio.Reader, cliente *clienttcp.Cliente) {
	// O servidor também confere a propriedade da reserva antes de cancelá-la.
	idReserva := lerTexto(leitor, "ID da reserva a cancelar: ")
	dados := protocol.CancelarReserva{IDReserva: idReserva}

	// Não há corpo específico na resposta de cancelamento, apenas sucesso/erro.
	resposta, err := cliente.Enviar("cancelar_reserva", dados, nil)
	if err != nil {
		fmt.Println("Erro de comunicação:", err)
		return
	}

	fmt.Println(resposta.Mensagem)
}

func mostrarItinerarios(itinerarios []protocol.ItinerarioEncontrado) {
	// Converte os valores internos em centavos para uma apresentação em reais.
	for indice, itinerario := range itinerarios {
		fmt.Printf("\nOpção %d - %s até %s\n", indice+1, itinerario.Origem, itinerario.Destino)
		fmt.Printf(
			"Preço total: R$ %d,%02d\n",
			itinerario.PrecoTotalCentavos/100,
			itinerario.PrecoTotalCentavos%100,
		)

		for _, trecho := range itinerario.Trechos {
			fmt.Printf(
				"- %s → %s - %s até %s - R$ %d,%02d - %d vaga(s)\n",
				trecho.Origem,
				trecho.Destino,
				formatarHorario(trecho.HorarioSaida),
				formatarHorario(trecho.HorarioChegada),
				trecho.PrecoCentavos/100,
				trecho.PrecoCentavos%100,
				trecho.AssentosDisponiveis,
			)
		}
	}
}

func lerDataDesejada(leitor *bufio.Reader) string {
	// O servidor recebe RFC3339, mas o terminal aceita um formato mais amigável.
	for {
		texto := lerTexto(leitor, "Data desejada para saída (dd/mm/aaaa): ")
		fusoBrasil := time.FixedZone("BRT", -3*60*60)
		data, err := time.ParseInLocation("02/01/2006", texto, fusoBrasil)
		if err != nil {
			fmt.Println("Data inválida; use dd/mm/aaaa.")
			continue
		}
		return data.Format(time.RFC3339)
	}
}

func formatarHorario(horarioRFC3339 string) string {
	// Faz o caminho inverso para apresentar no terminal uma data curta e legível.
	horario, err := time.Parse(time.RFC3339, horarioRFC3339)
	if err != nil {
		return horarioRFC3339
	}
	return horario.Format("02/01 15:04")
}

func novoIDReserva() string {
	// O gerador criptograficamente aleatório torna colisões extremamente improváveis.
	bytesAleatorios := make([]byte, 8)
	_, err := rand.Read(bytesAleatorios)
	if err == nil {
		return "reserva-" + hex.EncodeToString(bytesAleatorios)
	}

	return fmt.Sprintf("reserva-%d", time.Now().UnixNano())
}

func lerInteiroPositivo(leitor *bufio.Reader, pergunta string) int {
	// Reutiliza a leitura não negativa e restringe os campos que não aceitam zero.
	for {
		valor := lerInteiroNaoNegativo(leitor, pergunta)
		if valor > 0 {
			return valor
		}

		fmt.Println("Digite um número maior que zero.")
	}
}

func lerInteiroNaoNegativo(leitor *bufio.Reader, pergunta string) int {
	for {
		texto := lerTexto(leitor, pergunta)
		valor, err := strconv.Atoi(texto)
		if err != nil || valor < 0 {
			fmt.Println("Digite um número inteiro não negativo.")
			continue
		}

		return valor
	}
}

func lerTexto(leitor *bufio.Reader, pergunta string) string {
	// Repetir a pergunta dá ao terminal uma validação simples de campos vazios.
	for {
		fmt.Print(pergunta)

		texto, err := leitor.ReadString('\n')
		if err != nil {
			log.Fatalf("não foi possível ler a entrada: %v", err)
		}

		texto = strings.TrimSpace(texto)
		if texto == "" {
			fmt.Println("Este campo não pode ficar vazio.")
			continue
		}

		return texto
	}
}
