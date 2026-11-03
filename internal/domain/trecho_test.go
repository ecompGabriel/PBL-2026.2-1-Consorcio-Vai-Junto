package domain

import "testing"

func TestTrechoReservarDiminuiAssentos(t *testing.T) {
	trecho := Trecho{
		Capacidade:          4,
		AssentosDisponiveis: 4,
	}

	err := trecho.Reservar(2)

	if err != nil {
		t.Fatalf("não esperava erro ao reservar: %v", err)
	}

	if trecho.AssentosDisponiveis != 2 {
		t.Errorf(
			"assentos disponíveis = %d; esperava 2", trecho.AssentosDisponiveis,
		)
	}
}

func TestTrechoTemVagas(t *testing.T) {
	trecho := Trecho{
		Capacidade:          4,
		AssentosDisponiveis: 1,
	}

	temVagaParaUmPassageiro := trecho.TemVagas(1)

	if !temVagaParaUmPassageiro {
		t.Error("esperava vaga para 1 passageiro")
	}

	temVagaParaDoisPassageiros := trecho.TemVagas(2)

	if temVagaParaDoisPassageiros {
		t.Error("não esperava vaga para 2 passageiros")
	}
}

func TestTrechoReservarSemVagasNaoAlteraDisponibilidade(t *testing.T) {
	// Falhar ao reservar não pode reduzir a disponibilidade nem criar assentos
	// negativos; esta é a invariante local básica de um trecho.
	trecho := Trecho{
		Capacidade:          4,
		AssentosDisponiveis: 1,
	}

	err := trecho.Reservar(2)
	if err == nil {
		t.Fatalf("esperava erro ao reservar mais assentos do que o disponível")
	}

	if trecho.AssentosDisponiveis != 1 {
		t.Errorf(
			"assentos disponíveis = %d; esperava 1 após reserva recusada",
			trecho.AssentosDisponiveis,
		)
	}
}

func TestTrechoCancelarReservaDevolveAssentos(t *testing.T) {
	trecho := Trecho{
		Capacidade:          4,
		AssentosDisponiveis: 2,
	}

	err := trecho.CancelarReserva(2)
	if err != nil {
		t.Fatalf("nao esperava erro ao cancelar")
	}

	if trecho.AssentosDisponiveis != 4 {
		t.Errorf(
			"assentos disponíveis = %d; esperava 4", trecho.AssentosDisponiveis,
		)
	}
}

func TestTrechoCancelarReservaNaoUltrapassaCapacidade(t *testing.T) {
	// Um segundo cancelamento não pode fabricar vagas acima da capacidade original.
	trecho := Trecho{
		Capacidade:          4,
		AssentosDisponiveis: 3,
	}

	err := trecho.CancelarReserva(2)
	if err == nil {
		t.Fatalf("esperava erro ao cancelar reserva de assentos além da capacidade")
	}

	if trecho.AssentosDisponiveis != 3 {
		t.Errorf(
			"assentos disponíveis = %d; esperava 3 após cancelamento recusado", trecho.AssentosDisponiveis,
		)
	}
}
