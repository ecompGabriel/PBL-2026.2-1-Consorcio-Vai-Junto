package consorcio

import (
	"time"

	"vaijunto/internal/domain"
)

// TrechoDTO é o formato de dados que trafega na rede HTTP entre as empresas do consórcio.
type TrechoDTO struct {
	EmpresaID           string `json:"empresa_id"`
	CaronaID            string `json:"carona_id"`
	Ordem               int    `json:"ordem"`
	Origem              string `json:"origem"`
	Destino             string `json:"destino"`
	Capacidade          int    `json:"capacidade"`
	AssentosDisponiveis int    `json:"assentos_disponiveis"`
	PrecoCentavos       int64  `json:"preco_centavos"`
	HorarioSaida        string `json:"horario_saida"`   // Formatado em RFC3339
	HorarioChegada      string `json:"horario_chegada"` // Formatado em RFC3339
}

// ReferenciaTrechoDTO identifica um trecho em uma transação distribuída
type ReferenciaTrechoDTO struct {
	EmpresaID string `json:"empresa_id"`
	CaronaID  string `json:"carona_id"`
	Ordem     int    `json:"ordem"`
}

// PrepararReservaRequest é o payload enviado na Fase 1 do 2PC (Prepare)
type PrepararReservaRequest struct {
	TransacaoID        string                `json:"transacao_id"`
	PassageiroID       string                `json:"passageiro_id"`
	QuantidadeAssentos int                   `json:"quantidade_assentos"`
	Trechos            []ReferenciaTrechoDTO `json:"trechos"`
	TTLMilissegundos   int64                 `json:"ttl_ms"`
}

// RespostaTransacao é a resposta padrão para Prepare, Commit e Abort
type RespostaTransacao struct {
	Sucesso  bool   `json:"sucesso"`
	Mensagem string `json:"mensagem,omitempty"`
}

// ConverterParaDTO converte a entidade domain.Trecho local para o formato serializável TrechoDTO.
func ConverterParaDTO(t domain.Trecho) TrechoDTO {
	return TrechoDTO{
		EmpresaID:           t.EmpresaID,
		CaronaID:            t.CaronaID,
		Ordem:               t.Ordem,
		Origem:              t.Origem,
		Destino:             t.Destino,
		Capacidade:          t.Capacidade,
		AssentosDisponiveis: t.AssentosDisponiveis,
		PrecoCentavos:       t.PrecoCentavos,
		HorarioSaida:        t.HorarioSaida.Format(time.RFC3339),
		HorarioChegada:      t.HorarioChegada.Format(time.RFC3339),
	}
}

// ConverterDTOParaTrecho faz o caminho inverso: converte o TrechoDTO recebido via HTTP
// de volta para a entidade domain.Trecho, para ser usada no grafo de busca DFS.
func ConverterDTOParaTrecho(dto TrechoDTO) (domain.Trecho, error) {
	saida, err := time.Parse(time.RFC3339, dto.HorarioSaida)
	if err != nil {
		return domain.Trecho{}, err
	}

	chegada, err := time.Parse(time.RFC3339, dto.HorarioChegada)
	if err != nil {
		return domain.Trecho{}, err
	}

	return domain.Trecho{
		EmpresaID:           dto.EmpresaID,
		CaronaID:            dto.CaronaID,
		Ordem:               dto.Ordem,
		Origem:              dto.Origem,
		Destino:             dto.Destino,
		Capacidade:          dto.Capacidade,
		AssentosDisponiveis: dto.AssentosDisponiveis,
		PrecoCentavos:       dto.PrecoCentavos,
		HorarioSaida:        saida,
		HorarioChegada:      chegada,
	}, nil
}
