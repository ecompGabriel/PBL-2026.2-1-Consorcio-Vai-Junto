package protocol

import (
	"encoding/json"
	"testing"
)

func TestRequisicaoCarregaDadosDeCriarCarona(t *testing.T) {
	dadosOriginais := CriarCarona{
		ID:              "carona-1",
		HorarioSaida:    "2026-09-17T08:00:00-03:00",
		Rota:            []string{"Feira", "Alagoinhas", "Salvador"},
		Capacidade:      4,
		PrecosCentavos:  []int64{1500, 2000},
		DuracoesMinutos: []int{60, 90},
	}

	dadosJSON, err := json.Marshal(dadosOriginais)

	if err != nil {
		t.Fatalf("não esperava erro ao transformar os dados em JSON: %v", err)
	}

	requisicaoOriginal := Requisicao{
		Versao:   VersaoAtual,
		ID:       "req-1",
		Operacao: "criar_carona",
		Dados:    dadosJSON,
	}

	mensagemJSON, err := json.Marshal(requisicaoOriginal)

	if err != nil {
		t.Fatalf("não esperava erro ao transformar a requisição em JSON: %v", err)
	}

	var requisicaoRecebida Requisicao

	err = json.Unmarshal(mensagemJSON, &requisicaoRecebida)

	if err != nil {
		t.Fatalf("não esperava erro ao ler a requisição em JSON: %v", err)
	}

	var dadosRecebidos CriarCarona

	err = json.Unmarshal(requisicaoRecebida.Dados, &dadosRecebidos)

	if err != nil {
		t.Fatalf("não esperava erro ao ler os dados da carona: %v", err)
	}

	if requisicaoRecebida.Operacao != "criar_carona" {
		t.Errorf(
			"operação = %s; esperava criar_carona",
			requisicaoRecebida.Operacao,
		)
	}

	if dadosRecebidos.ID != dadosOriginais.ID {
		t.Errorf(
			"id = %s; esperava %s",
			dadosRecebidos.ID,
			dadosOriginais.ID,
		)
	}

	if dadosRecebidos.HorarioSaida != dadosOriginais.HorarioSaida {
		t.Errorf(
			"horário de saída = %s; esperava %s",
			dadosRecebidos.HorarioSaida,
			dadosOriginais.HorarioSaida,
		)
	}

	if dadosRecebidos.Capacidade != dadosOriginais.Capacidade {
		t.Errorf(
			"capacidade = %d; esperava %d",
			dadosRecebidos.Capacidade,
			dadosOriginais.Capacidade,
		)
	}

	if len(dadosRecebidos.Rota) != len(dadosOriginais.Rota) {
		t.Fatalf(
			"quantidade de cidades = %d; esperava %d",
			len(dadosRecebidos.Rota),
			len(dadosOriginais.Rota),
		)
	}

	for i, cidadeOriginal := range dadosOriginais.Rota {
		if dadosRecebidos.Rota[i] != cidadeOriginal {
			t.Errorf(
				"cidade na posição %d = %s; esperava %s",
				i,
				dadosRecebidos.Rota[i],
				cidadeOriginal,
			)
		}
	}

	if len(dadosRecebidos.PrecosCentavos) != len(dadosOriginais.PrecosCentavos) {
		t.Fatalf(
			"quantidade de preços = %d; esperava %d",
			len(dadosRecebidos.PrecosCentavos),
			len(dadosOriginais.PrecosCentavos),
		)
	}

	for i, precoOriginal := range dadosOriginais.PrecosCentavos {
		if dadosRecebidos.PrecosCentavos[i] != precoOriginal {
			t.Errorf(
				"preço na posição %d = %d; esperava %d",
				i,
				dadosRecebidos.PrecosCentavos[i],
				precoOriginal,
			)
		}
	}

	if len(dadosRecebidos.DuracoesMinutos) != len(dadosOriginais.DuracoesMinutos) {
		t.Fatalf("quantidade de durações = %d; esperava %d", len(dadosRecebidos.DuracoesMinutos), len(dadosOriginais.DuracoesMinutos))
	}
}

func TestDecodificarEstritoRejeitaCampoDesconhecido(t *testing.T) {
	dados := []byte(`{"origem":"Feira","destino":"Salvador","quantidade_assentos":1,"campo_extra":true}`)

	var busca BuscarItinerarios
	err := DecodificarEstrito(dados, &busca)
	if err == nil {
		t.Error("esperava erro para campo desconhecido")
	}
}
