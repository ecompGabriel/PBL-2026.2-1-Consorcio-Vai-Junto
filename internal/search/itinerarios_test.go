package search

import (
	"testing"
	"time"

	"vaijunto/internal/domain"
)

func TestBuscarItinerariosEncontraCaminhoComDoisTrechos(t *testing.T) {
	// Verifica a capacidade central do grafo: combinar trechos de caronas
	// diferentes em uma única opção de viagem para o passageiro.
	trechos := []domain.Trecho{
		{
			CaronaID:            "carona-1",
			Ordem:               1,
			Origem:              "Feira",
			Destino:             "Alagoinhas",
			Capacidade:          3,
			AssentosDisponiveis: 3,
			PrecoCentavos:       1000,
		},
		{
			CaronaID:            "carona-1",
			Ordem:               2,
			Origem:              "Alagoinhas",
			Destino:             "Salvador",
			Capacidade:          3,
			AssentosDisponiveis: 3,
			PrecoCentavos:       1500,
		},
	}

	itinerarios, err := BuscarItinerarios(trechosComHorarios(trechos), "Feira", "Salvador", 1, dataDeTeste())
	if err != nil {
		t.Fatalf("não esperava erro na busca: %v", err)
	}

	if len(itinerarios) != 1 {
		t.Fatalf("quantidade de itinerários = %d; esperava 1", len(itinerarios))
	}

	itinerario := itinerarios[0]

	if len(itinerario.Trechos) != 2 {
		t.Errorf("quantidade de trechos = %d; esperava 2", len(itinerario.Trechos))
	}

	if itinerario.PrecoTotalCentavos != 2500 {
		t.Errorf(
			"preço total = %d centavos; esperava 2500",
			itinerario.PrecoTotalCentavos,
		)
	}
}

func TestBuscarItinerariosOrdenaPeloMenorPreco(t *testing.T) {
	trechos := []domain.Trecho{
		{
			CaronaID:            "carona-cara",
			Origem:              "Feira",
			Destino:             "Salvador",
			Capacidade:          3,
			AssentosDisponiveis: 3,
			PrecoCentavos:       3000,
		},
		{
			CaronaID:            "carona-barata",
			Origem:              "Feira",
			Destino:             "Salvador",
			Capacidade:          3,
			AssentosDisponiveis: 3,
			PrecoCentavos:       2000,
		},
	}

	itinerarios, err := BuscarItinerarios(trechosComHorarios(trechos), "Feira", "Salvador", 1, dataDeTeste())
	if err != nil {
		t.Fatalf("não esperava erro na busca: %v", err)
	}

	if len(itinerarios) != 2 {
		t.Fatalf("quantidade de itinerários = %d; esperava 2", len(itinerarios))
	}

	if itinerarios[0].PrecoTotalCentavos != 2000 {
		t.Errorf(
			"primeiro itinerário custa %d centavos; esperava 2000",
			itinerarios[0].PrecoTotalCentavos,
		)
	}
}

func TestBuscarItinerariosIgnoraTrechoSemVagas(t *testing.T) {
	trechos := []domain.Trecho{
		{
			CaronaID:            "carona-1",
			Origem:              "Feira",
			Destino:             "Salvador",
			Capacidade:          2,
			AssentosDisponiveis: 0,
			PrecoCentavos:       2000,
		},
	}

	itinerarios, err := BuscarItinerarios(trechosComHorarios(trechos), "Feira", "Salvador", 2, dataDeTeste())
	if err != nil {
		t.Fatalf("não esperava erro na busca: %v", err)
	}

	if len(itinerarios) != 0 {
		t.Errorf("quantidade de itinerários = %d; esperava 0", len(itinerarios))
	}
}

func TestBuscarItinerariosNaoDiferenciaMaiusculasEMinusculas(t *testing.T) {
	trechos := []domain.Trecho{
		{
			CaronaID:            "carona-1",
			Origem:              "Feira",
			Destino:             "Salvador",
			Capacidade:          3,
			AssentosDisponiveis: 3,
			PrecoCentavos:       2000,
		},
	}

	itinerarios, err := BuscarItinerarios(trechosComHorarios(trechos), "  FEIRA ", "sAlVaDoR", 1, dataDeTeste())
	if err != nil {
		t.Fatalf("não esperava erro na busca: %v", err)
	}

	if len(itinerarios) != 1 {
		t.Fatalf("quantidade de itinerários = %d; esperava 1", len(itinerarios))
	}

	if itinerarios[0].Origem != "Feira" || itinerarios[0].Destino != "Salvador" {
		t.Errorf(
			"itinerário retornado = %s até %s; esperava Feira até Salvador",
			itinerarios[0].Origem,
			itinerarios[0].Destino,
		)
	}
}

func TestBuscarItinerariosRejeitaQuantidadeInvalida(t *testing.T) {
	itinerarios, err := BuscarItinerarios(nil, "Feira", "Salvador", 0, dataDeTeste())

	if err == nil {
		t.Fatal("esperava erro para quantidade de assentos igual a zero")
	}

	if itinerarios != nil {
		t.Errorf("itinerários = %v; esperava nil", itinerarios)
	}
}

func TestBuscarItinerariosAceitaConexaoComEsperaDeTrintaMinutos(t *testing.T) {
	// Uma troca de carona precisa respeitar exatamente a conexão mínima definida
	// pela aplicação, evitando itinerários que já teriam partido.
	data := dataDeTeste()
	trechos := []domain.Trecho{
		{CaronaID: "feira-alagoinhas", Origem: "Feira", Destino: "Alagoinhas", Capacidade: 2, AssentosDisponiveis: 2, PrecoCentavos: 1000, HorarioSaida: data.Add(8 * time.Hour), HorarioChegada: data.Add(10 * time.Hour)},
		{CaronaID: "alagoinhas-aracaju", Origem: "Alagoinhas", Destino: "Aracaju", Capacidade: 2, AssentosDisponiveis: 2, PrecoCentavos: 2500, HorarioSaida: data.Add(10*time.Hour + 30*time.Minute), HorarioChegada: data.Add(16 * time.Hour)},
	}

	itinerarios, err := BuscarItinerarios(trechos, "Feira", "Aracaju", 1, data)
	if err != nil {
		t.Fatalf("não esperava erro na busca: %v", err)
	}
	if len(itinerarios) != 1 {
		t.Fatalf("quantidade de itinerários = %d; esperava 1", len(itinerarios))
	}
}

func TestBuscarItinerariosRejeitaConexaoQueJaPartiu(t *testing.T) {
	// O segundo trecho sai antes da chegada/conexão necessária e, portanto, não
	// pode aparecer como uma continuação possível do primeiro.
	data := dataDeTeste()
	trechos := []domain.Trecho{
		{CaronaID: "feira-alagoinhas", Origem: "Feira", Destino: "Alagoinhas", Capacidade: 2, AssentosDisponiveis: 2, PrecoCentavos: 1000, HorarioSaida: data.Add(8 * time.Hour), HorarioChegada: data.Add(10 * time.Hour)},
		{CaronaID: "alagoinhas-aracaju", Origem: "Alagoinhas", Destino: "Aracaju", Capacidade: 2, AssentosDisponiveis: 2, PrecoCentavos: 2500, HorarioSaida: data.Add(9 * time.Hour), HorarioChegada: data.Add(15 * time.Hour)},
	}

	itinerarios, err := BuscarItinerarios(trechos, "Feira", "Aracaju", 1, data)
	if err != nil {
		t.Fatalf("não esperava erro na busca: %v", err)
	}
	if len(itinerarios) != 0 {
		t.Errorf("quantidade de itinerários = %d; esperava 0", len(itinerarios))
	}
}

func TestBuscarItinerariosAceitaEsperaDeDiasEntreCaronas(t *testing.T) {
	data := dataDeTeste()
	trechos := []domain.Trecho{
		{CaronaID: "feira-alagoinhas", Origem: "Feira", Destino: "Alagoinhas", Capacidade: 2, AssentosDisponiveis: 2, PrecoCentavos: 1000, HorarioSaida: data.Add(8 * time.Hour), HorarioChegada: data.Add(10 * time.Hour)},
		{CaronaID: "alagoinhas-aracaju", Origem: "Alagoinhas", Destino: "Aracaju", Capacidade: 2, AssentosDisponiveis: 2, PrecoCentavos: 2500, HorarioSaida: data.AddDate(0, 0, 2).Add(8 * time.Hour), HorarioChegada: data.AddDate(0, 0, 2).Add(14 * time.Hour)},
	}

	itinerarios, err := BuscarItinerarios(trechos, "Feira", "Aracaju", 1, data)
	if err != nil {
		t.Fatalf("não esperava erro na busca: %v", err)
	}
	if len(itinerarios) != 1 {
		t.Fatalf("quantidade de itinerários = %d; esperava 1", len(itinerarios))
	}
}

func dataDeTeste() time.Time {
	return time.Date(2026, time.September, 17, 0, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
}

func trechosComHorarios(trechos []domain.Trecho) []domain.Trecho {
	proximosHorarios := make(map[string]time.Time)
	data := dataDeTeste()

	for indice := range trechos {
		horarioSaida := proximosHorarios[trechos[indice].CaronaID]
		if horarioSaida.IsZero() {
			horarioSaida = data.Add(8 * time.Hour)
		}
		trechos[indice].HorarioSaida = horarioSaida
		trechos[indice].HorarioChegada = horarioSaida.Add(time.Hour)
		proximosHorarios[trechos[indice].CaronaID] = trechos[indice].HorarioChegada
	}

	return trechos
}
