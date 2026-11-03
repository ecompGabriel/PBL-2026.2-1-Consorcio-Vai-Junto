package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// VersaoAtual identifica o formato do protocolo aceito por cliente e servidor.
const VersaoAtual = "1.0"

const (
	PerfilMotorista  = "motorista"
	PerfilPassageiro = "passageiro"
)

type Requisicao struct {
	// Cabeçalho comum de toda mensagem enviada ao servidor.
	// Dados permanece em JSON bruto até o servidor identificar a operação e
	// decodificá-lo na estrutura específica correspondente.
	Versao   string          `json:"versao"`
	ID       string          `json:"id"`
	Operacao string          `json:"operacao"`
	Dados    json.RawMessage `json:"dados"`
}

type Resposta struct {
	// Cabeçalho comum de toda resposta enviada ao cliente.
	Versao   string          `json:"versao"`
	ID       string          `json:"id"`
	Sucesso  bool            `json:"sucesso"`
	Codigo   string          `json:"codigo"`
	Mensagem string          `json:"mensagem"`
	Dados    json.RawMessage `json:"dados,omitempty"`
}

type RegistrarUsuario struct {
	// O perfil é validado no servidor; o cliente não pode se cadastrar com um
	// valor arbitrário e ganhar permissões inexistentes.
	UsuarioID string `json:"usuario_id"`
	Senha     string `json:"senha"`
	Perfil    string `json:"perfil"`
}

type IniciarSessao struct {
	UsuarioID string `json:"usuario_id"`
	Senha     string `json:"senha"`
}

type SessaoIniciada struct {
	UsuarioID string `json:"usuario_id"`
	Perfil    string `json:"perfil"`
}

type CriarCarona struct {
	// Corpo específico da operação criar_carona.
	ID              string   `json:"id"`
	HorarioSaida    string   `json:"horario_saida"`
	Rota            []string `json:"rota"`
	Capacidade      int      `json:"capacidade"`
	PrecosCentavos  []int64  `json:"precos_centavos"`
	DuracoesMinutos []int    `json:"duracoes_minutos"`
}

type BuscarItinerarios struct {
	// Corpo específico da operação buscar_itinerarios.
	Origem             string `json:"origem"`
	Destino            string `json:"destino"`
	QuantidadeAssentos int    `json:"quantidade_assentos"`
	DataDesejada       string `json:"data_desejada"`
}

type ItinerarioEncontrado struct {
	Origem             string             `json:"origem"`
	Destino            string             `json:"destino"`
	Trechos            []TrechoEncontrado `json:"trechos"`
	PrecoTotalCentavos int64              `json:"preco_total_centavos"`
}

type TrechoEncontrado struct {
	EmpresaID           string `json:"empresa_id"`
	CaronaID            string `json:"carona_id"`
	Ordem               int    `json:"ordem"`
	Origem              string `json:"origem"`
	Destino             string `json:"destino"`
	PrecoCentavos       int64  `json:"preco_centavos"`
	AssentosDisponiveis int    `json:"assentos_disponiveis"`
	HorarioSaida        string `json:"horario_saida"`
	HorarioChegada      string `json:"horario_chegada"`
}

type ReferenciaTrecho struct {
	EmpresaID string `json:"empresa_id"`
	CaronaID  string `json:"carona_id"`
	Ordem     int    `json:"ordem"`
}

type ConfirmarReserva struct {
	// Corpo específico da operação confirmar_reserva.
	// Os trechos identificam a opção escolhida pelo passageiro na busca.
	IDReserva          string             `json:"id_reserva"`
	QuantidadeAssentos int                `json:"quantidade_assentos"`
	Trechos            []ReferenciaTrecho `json:"trechos"`
}

type CancelarReserva struct {
	// O passageiro é identificado pela sessão, portanto basta indicar a reserva.
	IDReserva string `json:"id_reserva"`
}

type CancelarCarona struct {
	// Também usa a identidade do motorista autenticado para verificar a posse.
	IDCarona string `json:"id_carona"`
}

type Reserva struct {
	// É a representação de saída: não expõe detalhes internos do domínio além
	// do necessário para o passageiro consultar ou cancelar a própria reserva.
	ID                 string             `json:"id"`
	PassageiroID       string             `json:"passageiro_id"`
	QuantidadeAssentos int                `json:"quantidade_assentos"`
	Status             string             `json:"status"`
	Trechos            []ReferenciaTrecho `json:"trechos"`
}

type PassageiroConfirmado struct {
	PassageiroID       string `json:"passageiro_id"`
	QuantidadeAssentos int    `json:"quantidade_assentos"`
}

type TrechoDoMotorista struct {
	Ordem               int                    `json:"ordem"`
	Origem              string                 `json:"origem"`
	Destino             string                 `json:"destino"`
	Capacidade          int                    `json:"capacidade"`
	AssentosDisponiveis int                    `json:"assentos_disponiveis"`
	HorarioSaida        string                 `json:"horario_saida"`
	HorarioChegada      string                 `json:"horario_chegada"`
	Passageiros         []PassageiroConfirmado `json:"passageiros"`
}

type CaronaDoMotorista struct {
	// É a visão de uma carona para o motorista, incluindo passageiros somente
	// dos trechos daquela carona.
	ID        string              `json:"id"`
	Cancelada bool                `json:"cancelada"`
	Trechos   []TrechoDoMotorista `json:"trechos"`
}

func DecodificarEstrito(dados []byte, destino any) error {
	// Além de decodificar, rejeita campos não previstos e outro JSON após o
	// primeiro valor. Isso evita aceitar mensagens ambíguas no protocolo.
	decodificador := json.NewDecoder(bytes.NewReader(dados))
	decodificador.DisallowUnknownFields()

	err := decodificador.Decode(destino)
	if err != nil {
		return err
	}

	err = decodificador.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return errors.New("a mensagem contém dados JSON adicionais")
	}

	return nil
}
