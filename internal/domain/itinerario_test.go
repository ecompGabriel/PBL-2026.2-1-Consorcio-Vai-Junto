package domain

import "testing"

func TestItinerarioCalcularPrecoTotal(t *testing.T) {
	itinerario := Itinerario{
		Trechos: []Trecho{
			{PrecoCentavos: 1500},
			{PrecoCentavos: 2000},
		},
	}

	precoTotal := itinerario.CalcularPrecoTotal()

	if precoTotal != 3500 {
		t.Errorf("preço total = %d centavos; esperava 3500", precoTotal)
	}
}

func TestItinerarioTemVagas(t *testing.T) {
	itinerario := Itinerario{
		Trechos: []Trecho{
			{Capacidade: 4, AssentosDisponiveis: 2},
			{Capacidade: 4, AssentosDisponiveis: 3},
		},
	}

	if !itinerario.TemVagas(2) {
		t.Error("esperava vagas para 2 passageiros em todos os trechos")
	}

	itinerario.Trechos[1].AssentosDisponiveis = 1
	if itinerario.TemVagas(2) {
		t.Error("não esperava vagas quando um trecho tem apenas 1 assento disponível")
	}
}
