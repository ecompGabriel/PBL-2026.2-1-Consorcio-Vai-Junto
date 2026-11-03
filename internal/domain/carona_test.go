package domain

import (
	"testing"
	"time"
)

func caronaDeTeste() Carona {
	return Carona{
		ID:        "carona-1",
		empresaID: "empresa-1",
		rota:      []string{"Feira", "Alagoinhas", "Salvador"},
		trechos: []Trecho{
			{
				EmpresaID:           "empresa-1",
				CaronaID:            "carona-1",
				Ordem:               0,
				Origem:              "Feira",
				Destino:             "Alagoinhas",
				Capacidade:          4,
				AssentosDisponiveis: 4,
				PrecoCentavos:       1500,
			},
			{
				EmpresaID:           "empresa-1",
				CaronaID:            "carona-1",
				Ordem:               1,
				Origem:              "Alagoinhas",
				Destino:             "Salvador",
				Capacidade:          4,
				AssentosDisponiveis: 4,
				PrecoCentavos:       2000,
			},
		},
	}
}

func TestCaronaTemCidade(t *testing.T) {
	carona := caronaDeTeste()

	if !carona.TemCidade("Alagoinhas") {
		t.Error("esperava encontrar Alagoinhas na rota")
	}

	if carona.TemCidade("Camaçari") {
		t.Error("não esperava encontrar Camaçari na rota")
	}
}

func TestCaronaTrechosDevolveCopia(t *testing.T) {
	carona := caronaDeTeste()

	trechos := carona.Trechos()
	if len(trechos) != 2 {
		t.Fatalf("quantidade de trechos = %d; esperava 2", len(trechos))
	}

	trechos[0].Origem = "Outra cidade"
	if carona.trechos[0].Origem == "Outra cidade" {
		t.Error("alterar a cópia dos trechos não deveria alterar a carona")
	}
}

func TestCaronaReservaEDevolveAssentosNoTrechoPorOrdem(t *testing.T) {
	carona := caronaDeTeste()

	err := carona.ReservarNoTrecho(1, 2)
	if err != nil {
		t.Fatalf("não esperava erro ao reservar: %v", err)
	}

	if carona.trechos[0].AssentosDisponiveis != 4 {
		t.Errorf(
			"assentos do trecho 0 = %d; esperava 4",
			carona.trechos[0].AssentosDisponiveis,
		)
	}

	if carona.trechos[1].AssentosDisponiveis != 2 {
		t.Errorf(
			"assentos do trecho 1 = %d; esperava 2",
			carona.trechos[1].AssentosDisponiveis,
		)
	}

	err = carona.CancelarNoTrecho(1, 2)
	if err != nil {
		t.Fatalf("não esperava erro ao cancelar: %v", err)
	}

	if carona.trechos[1].AssentosDisponiveis != 4 {
		t.Errorf(
			"assentos do trecho 1 = %d; esperava 4",
			carona.trechos[1].AssentosDisponiveis,
		)
	}
}

func TestCaronaRejeitaOrdemDeTrechoInexistente(t *testing.T) {
	carona := caronaDeTeste()

	err := carona.ReservarNoTrecho(10, 1)
	if err == nil {
		t.Error("esperava erro para ordem de trecho inexistente")
	}

	if carona.TemVagasNoTrecho(10, 1) {
		t.Error("não esperava vagas para trecho inexistente")
	}
}

func TestCaronaPosicaoCidade(t *testing.T) {
	carona := caronaDeTeste()

	posicao, encontrou := carona.PosicaoCidade("Salvador")
	if !encontrou {
		t.Fatal("esperava encontrar Salvador na rota")
	}

	if posicao != 2 {
		t.Errorf("posição de Salvador = %d; esperava 2", posicao)
	}

	_, encontrou = carona.PosicaoCidade("Camaçari")
	if encontrou {
		t.Error("não esperava encontrar Camaçari na rota")
	}
}

func TestCaronaTrechosEntre(t *testing.T) {
	carona := caronaDeTeste()

	trechos, err := carona.TrechosEntre("Feira", "Salvador")
	if err != nil {
		t.Fatalf("não esperava erro ao buscar trechos: %v", err)
	}

	if len(trechos) != 2 {
		t.Fatalf("quantidade de trechos = %d; esperava 2", len(trechos))
	}

	if trechos[0].Origem != "Feira" || trechos[0].Destino != "Alagoinhas" {
		t.Error("primeiro trecho não é Feira para Alagoinhas")
	}

	if trechos[1].Origem != "Alagoinhas" || trechos[1].Destino != "Salvador" {
		t.Error("segundo trecho não é Alagoinhas para Salvador")
	}
}

func TestCaronaTrechosEntreRejeitaCidadesInvalidas(t *testing.T) {
	carona := caronaDeTeste()

	_, err := carona.TrechosEntre("Feira", "Camaçari")
	if err == nil {
		t.Error("esperava erro para destino fora da rota")
	}

	_, err = carona.TrechosEntre("Salvador", "Feira")
	if err == nil {
		t.Error("esperava erro quando a origem vem depois do destino")
	}
}

func TestCaronaPrecosEntre(t *testing.T) {
	carona := caronaDeTeste()

	preco, err := carona.PrecosEntre("Feira", "Salvador")
	if err != nil {
		t.Fatalf("não esperava erro ao calcular preço: %v", err)
	}

	if preco != 3500 {
		t.Errorf("preço = %d centavos; esperava 3500", preco)
	}
}

func TestNovaCaronaCriaTrechos(t *testing.T) {
	horario := time.Date(2026, time.September, 17, 8, 0, 0, 0, time.UTC)

	carona, err := NovaCarona(
		"empresa-1",
		"carona-1",
		"motorista-1",
		horario,
		[]string{"Feira", "Alagoinhas", "Salvador"},
		4,
		[]int64{1500, 2000},
		[]int{60, 90},
	)
	if err != nil {
		t.Fatalf("não esperava erro ao criar carona: %v", err)
	}

	if carona == nil {
		t.Fatal("esperava uma carona criada")
	}

	if carona.EmpresaID() != "empresa-1" {
		t.Errorf("empresa = %s; esperava empresa-1", carona.EmpresaID())
	}

	if len(carona.trechos) != 2 {
		t.Fatalf("quantidade de trechos = %d; esperava 2", len(carona.trechos))
	}

	primeiroTrecho := carona.trechos[0]
	if primeiroTrecho.EmpresaID != "empresa-1" || primeiroTrecho.Ordem != 0 || primeiroTrecho.Origem != "Feira" || primeiroTrecho.Destino != "Alagoinhas" {
		t.Error("primeiro trecho foi criado com dados incorretos")
	}

	if primeiroTrecho.Capacidade != 4 || primeiroTrecho.AssentosDisponiveis != 4 || primeiroTrecho.PrecoCentavos != 1500 {
		t.Error("capacidade, disponibilidade ou preço do primeiro trecho estão incorretos")
	}
	if !primeiroTrecho.HorarioSaida.Equal(horario) || !primeiroTrecho.HorarioChegada.Equal(horario.Add(time.Hour)) {
		t.Error("horários do primeiro trecho estão incorretos")
	}

	segundoTrecho := carona.trechos[1]
	if segundoTrecho.EmpresaID != "empresa-1" || segundoTrecho.Ordem != 1 || segundoTrecho.Origem != "Alagoinhas" || segundoTrecho.Destino != "Salvador" || segundoTrecho.PrecoCentavos != 2000 {
		t.Error("segundo trecho foi criado com dados incorretos")
	}
}

func TestNovaCaronaRejeitaEmpresaInvalida(t *testing.T) {
	_, err := NovaCarona(
		"",
		"carona-1",
		"motorista-1",
		time.Now(),
		[]string{"Feira", "Salvador"},
		4,
		[]int64{2000},
		[]int{60},
	)

	if err == nil {
		t.Error("esperava erro para ID de empresa vazio")
	}
}

func TestNovaCaronaRejeitaRotaInvalida(t *testing.T) {
	_, err := NovaCarona(
		"empresa-1",
		"carona-1",
		"motorista-1",
		time.Now(),
		[]string{"Feira"},
		4,
		[]int64{},
		[]int{},
	)

	if err == nil {
		t.Error("esperava erro para rota com menos de duas cidades")
	}
}

func TestNovaCaronaRejeitaCapacidadeInvalida(t *testing.T) {
	_, err := NovaCarona(
		"empresa-1",
		"carona-1",
		"motorista-1",
		time.Now(),
		[]string{"Feira", "Salvador"},
		0,
		[]int64{2000},
		[]int{60},
	)

	if err == nil {
		t.Error("esperava erro para capacidade igual a zero")
	}
}

func TestNovaCaronaRejeitaPrecosInvalidos(t *testing.T) {
	_, err := NovaCarona(
		"empresa-1",
		"carona-1",
		"motorista-1",
		time.Now(),
		[]string{"Feira", "Alagoinhas", "Salvador"},
		4,
		[]int64{1500},
		[]int{60, 90},
	)

	if err == nil {
		t.Error("esperava erro para quantidade de preços diferente da quantidade de trechos")
	}

	_, err = NovaCarona(
		"empresa-1",
		"carona-1",
		"motorista-1",
		time.Now(),
		[]string{"Feira", "Salvador"},
		4,
		[]int64{-1},
		[]int{60},
	)

	if err == nil {
		t.Error("esperava erro para preço negativo")
	}
}

func TestNovaCaronaRejeitaDuracoesInvalidas(t *testing.T) {
	_, err := NovaCarona(
		"empresa-1",
		"carona-1",
		"motorista-1",
		time.Now(),
		[]string{"Feira", "Alagoinhas", "Salvador"},
		4,
		[]int64{1500, 2000},
		[]int{60},
	)
	if err == nil {
		t.Error("esperava erro para quantidade de durações diferente da quantidade de trechos")
	}

	_, err = NovaCarona(
		"empresa-1",
		"carona-1",
		"motorista-1",
		time.Now(),
		[]string{"Feira", "Salvador"},
		4,
		[]int64{2000},
		[]int{0},
	)
	if err == nil {
		t.Error("esperava erro para duração igual a zero")
	}
}
