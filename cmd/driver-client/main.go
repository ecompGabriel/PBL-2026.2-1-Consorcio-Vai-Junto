package main

import (
	"bufio"
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
	// O cliente motorista mantém uma única conexão TCP e apresenta o menu após
	// a autenticação bem-sucedida nessa conexão.
	leitor := bufio.NewReader(os.Stdin)

	cliente, err := clienttcp.Conectar()
	if err != nil {
		log.Fatalf("não foi possível conectar ao servidor: %v", err)
	}
	defer cliente.Fechar()

	if !autenticar(leitor, cliente, protocol.PerfilMotorista) {
		return
	}

	for {
		fmt.Println("\nVaiJunto - Motorista")
		fmt.Println("1 - Publicar carona")
		fmt.Println("2 - Consultar caronas e passageiros")
		fmt.Println("3 - Cancelar carona")
		fmt.Println("0 - Sair")

		opcao := lerTexto(leitor, "Escolha: ")
		switch opcao {
		case "1":
			publicarCarona(leitor, cliente)
		case "2":
			consultarCaronas(cliente)
		case "3":
			cancelarCarona(leitor, cliente)
		case "0":
			fmt.Println("Até logo!")
			return
		default:
			fmt.Println("Opção inválida.")
		}
	}
}

func autenticar(leitor *bufio.Reader, cliente *clienttcp.Cliente, perfil string) bool {
	// Erros de cadastro e senha não encerram o programa: o usuário pode tentar
	// novamente ou escolher sair no menu inicial.
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

func publicarCarona(leitor *bufio.Reader, cliente *clienttcp.Cliente) {
	// Coleta os dados amigáveis do terminal e monta o corpo da operação remota.
	id := lerTexto(leitor, "ID da carona: ")
	horarioSaida := lerHorarioSaida(leitor)
	rota := lerRota(leitor)
	capacidade := lerCapacidade(leitor)
	precosCentavos := lerPrecosCentavos(leitor, rota)
	duracoesMinutos := lerDuracoesMinutos(leitor, rota)

	dados := protocol.CriarCarona{
		ID:              id,
		HorarioSaida:    horarioSaida,
		Rota:            rota,
		Capacidade:      capacidade,
		PrecosCentavos:  precosCentavos,
		DuracoesMinutos: duracoesMinutos,
	}

	resposta, err := cliente.Enviar("criar_carona", dados, nil)
	if err != nil {
		fmt.Println("Erro de comunicação:", err)
		return
	}

	fmt.Println(resposta.Mensagem)
}

func consultarCaronas(cliente *clienttcp.Cliente) {
	// O servidor devolve somente as caronas da sessão do motorista autenticado.
	var caronas []protocol.CaronaDoMotorista
	resposta, err := cliente.Enviar("listar_caronas_motorista", struct{}{}, &caronas)
	if err != nil {
		fmt.Println("Erro de comunicação:", err)
		return
	}
	if !resposta.Sucesso {
		fmt.Println("Consulta recusada:", resposta.Mensagem)
		return
	}

	if len(caronas) == 0 {
		fmt.Println("Você ainda não publicou caronas.")
		return
	}

	for _, carona := range caronas {
		// A carona cancelada é exibida, e não escondida, para que o motorista
		// diferencie um cancelamento anterior de uma carona nunca publicada.
		fmt.Printf("\nCarona %s\n", carona.ID)
		if carona.Cancelada {
			fmt.Println("Status: cancelada")
		} else {
			fmt.Println("Status: ativa")
		}
		for _, trecho := range carona.Trechos {
			fmt.Printf(
				"- Trecho %d: %s → %s - %d/%d vaga(s) - %s até %s\n",
				trecho.Ordem,
				trecho.Origem,
				trecho.Destino,
				trecho.AssentosDisponiveis,
				trecho.Capacidade,
				formatarHorario(trecho.HorarioSaida),
				formatarHorario(trecho.HorarioChegada),
			)

			if len(trecho.Passageiros) == 0 {
				fmt.Println("  Nenhum passageiro confirmado.")
				continue
			}

			for _, passageiro := range trecho.Passageiros {
				fmt.Printf(
					"  Passageiro %s: %d assento(s)\n",
					passageiro.PassageiroID,
					passageiro.QuantidadeAssentos,
				)
			}
		}
	}

}

func cancelarCarona(leitor *bufio.Reader, cliente *clienttcp.Cliente) {
	// A autorização real acontece no servidor a partir da sessão; o cliente só
	// informa qual das próprias caronas o motorista deseja cancelar.
	idCarona := lerTexto(leitor, "ID da carona a cancelar: ")

	resposta, err := cliente.Enviar("cancelar_carona", protocol.CancelarCarona{
		IDCarona: idCarona,
	}, nil)
	if err != nil {
		fmt.Println("Erro de comunicação:", err)
		return
	}
	if !resposta.Sucesso {
		fmt.Println("Cancelamento recusado:", resposta.Mensagem)
		return
	}

	fmt.Println(resposta.Mensagem)
}

func lerTexto(leitor *bufio.Reader, pergunta string) string {
	// Centraliza a leitura para impedir que campos obrigatórios sejam vazios.
	for {
		fmt.Print(pergunta)

		texto, err := leitor.ReadString('\n')
		if err != nil {
			log.Fatalf("não foi possível ler a entrada: %v", err)
		}

		texto = strings.TrimSpace(texto)
		if texto == "" {
			fmt.Println("Este campo não pode ficar vazio")
			continue
		}

		return texto
	}
}

func lerRota(leitor *bufio.Reader) []string {
	// A rota é montada cidade a cidade e preserva a ordem informada pelo motorista.
	origem := lerTexto(leitor, "Digite a cidade de origem da rota: ")
	rota := []string{origem}

	for {
		adiciona := lerTexto(leitor, "Deseja adicionar mais uma cidade à rota? (1) Sim (2) Não: ")
		if adiciona == "1" {
			novaCidade := lerTexto(leitor, "Digite a próxima cidade: ")
			rota = append(rota, novaCidade)
		} else if adiciona == "2" {
			if len(rota) < 2 {
				fmt.Println("A rota precisa ter ao menos duas cidades")
				continue
			}

			return rota
		} else {
			fmt.Println("Digite uma resposta válida")
		}
	}
}

func lerCapacidade(leitor *bufio.Reader) int {
	// A capacidade precisa ser positiva, pois cada trecho nasce com esse mesmo
	// número de assentos disponíveis.
	for {
		textoCapacidade := lerTexto(leitor, "Capacidade do veículo: ")
		capacidade, err := strconv.Atoi(textoCapacidade)
		if err != nil || capacidade <= 0 {
			fmt.Println("Digite uma capacidade inteira maior que zero")
			continue
		}

		return capacidade
	}
}

func lerHorarioSaida(leitor *bufio.Reader) string {
	// A interface recebe data e hora separadamente; o protocolo envia RFC3339.
	for {
		data := lerTexto(leitor, "Data de saída (dd/mm/aaaa): ")
		hora := lerTexto(leitor, "Horário de saída (hh:mm): ")

		horarioSaida, err := converterDataHoraParaRFC3339(data, hora)
		if err != nil {
			fmt.Println("Data ou horário inválidos; use dd/mm/aaaa e hh:mm")
			continue
		}

		err = validarHorarioFuturo(horarioSaida, time.Now())
		if err != nil {
			fmt.Println(err)
			continue
		}

		return horarioSaida
	}
}

func converterDataHoraParaRFC3339(data string, hora string) (string, error) {
	// O layout de Go usa uma data de referência. O fuso BRT equivale a UTC-3.
	fusoBrasil := time.FixedZone("BRT", -3*60*60)
	horarioSaida, err := time.ParseInLocation(
		"02/01/2006 15:04",
		data+" "+hora,
		fusoBrasil,
	)
	if err != nil {
		return "", err
	}

	return horarioSaida.Format(time.RFC3339), nil
}

func validarHorarioFuturo(horarioRFC3339 string, agora time.Time) error {
	// Evita publicar no terminal uma viagem que já começou ou já passou.
	horarioSaida, err := time.Parse(time.RFC3339, horarioRFC3339)
	if err != nil {
		return err
	}

	if !horarioSaida.After(agora) {
		return fmt.Errorf("o horário de saída precisa estar no futuro")
	}

	return nil
}

func lerPrecosCentavos(leitor *bufio.Reader, rota []string) []int64 {
	// Há um preço para cada par de cidades consecutivas da rota.
	precosCentavos := make([]int64, 0, len(rota)-1)

	for i := 0; i < len(rota)-1; i++ {
		pergunta := fmt.Sprintf(
			"Preço de %s até %s em reais (ex.: 15,50): ",
			rota[i],
			rota[i+1],
		)

		for {
			textoPreco := lerTexto(leitor, pergunta)
			preco, err := converterReaisParaCentavos(textoPreco)
			if err != nil {
				fmt.Println("Preço inválido:", err)
				continue
			}

			precosCentavos = append(precosCentavos, preco)
			break
		}
	}

	return precosCentavos
}

func lerDuracoesMinutos(leitor *bufio.Reader, rota []string) []int {
	// Como a rota tem N cidades, há N-1 durações: uma para cada par consecutivo.
	duracoes := make([]int, 0, len(rota)-1)

	for i := 0; i < len(rota)-1; i++ {
		pergunta := fmt.Sprintf(
			"Duração estimada de %s até %s, em minutos: ",
			rota[i],
			rota[i+1],
		)
		duracoes = append(duracoes, lerInteiroPositivo(leitor, pergunta))
	}

	return duracoes
}

func lerInteiroPositivo(leitor *bufio.Reader, pergunta string) int {
	for {
		texto := lerTexto(leitor, pergunta)
		valor, err := strconv.Atoi(texto)
		if err != nil || valor <= 0 {
			fmt.Println("Digite um número inteiro maior que zero.")
			continue
		}
		return valor
	}
}

func formatarHorario(horarioRFC3339 string) string {
	horario, err := time.Parse(time.RFC3339, horarioRFC3339)
	if err != nil {
		return horarioRFC3339
	}
	return horario.Format("02/01 15:04")
}

func converterReaisParaCentavos(textoPreco string) (int64, error) {
	// Aceita vírgula ou ponto na entrada, mas armazena sempre um inteiro em
	// centavos para que operações financeiras sejam exatas.
	valor := strings.TrimSpace(textoPreco)
	partes := strings.Split(strings.ReplaceAll(valor, ",", "."), ".")

	if len(partes) > 2 || partes[0] == "" {
		return 0, fmt.Errorf("use um valor como 15,50")
	}

	reais, err := strconv.ParseInt(partes[0], 10, 64)
	if err != nil || reais < 0 {
		return 0, fmt.Errorf("use um valor não negativo como 15,50")
	}

	centavos := int64(0)
	if len(partes) == 2 {
		if len(partes[1]) == 0 || len(partes[1]) > 2 {
			return 0, fmt.Errorf("use no máximo duas casas decimais")
		}

		centavos, err = strconv.ParseInt(partes[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("use um valor como 15,50")
		}

		if len(partes[1]) == 1 {
			centavos *= 10
		}
	}

	return reais*100 + centavos, nil
}
