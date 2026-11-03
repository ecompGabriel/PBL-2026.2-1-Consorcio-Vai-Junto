package store

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"vaijunto/internal/domain"
)

func novaCaronaDeTeste(t *testing.T, id string) *domain.Carona {
	t.Helper()

	carona, err := domain.NovaCarona(
		"empresa-1",
		id,
		"motorista-1",
		time.Date(2026, time.September, 17, 8, 0, 0, 0, time.UTC),
		[]string{"Feira", "Salvador"},
		4,
		[]int64{2000},
		[]int{60},
	)
	if err != nil {
		t.Fatalf("não esperava erro ao criar carona de teste: %v", err)
	}

	return carona
}

func novaCaronaComRotaDeTeste(t *testing.T, id string, capacidade int) *domain.Carona {
	t.Helper()

	carona, err := domain.NovaCarona(
		"empresa-1",
		id,
		"motorista-1",
		time.Date(2026, time.September, 17, 8, 0, 0, 0, time.UTC),
		[]string{"Feira", "Alagoinhas", "Salvador"},
		capacidade,
		[]int64{1000, 1500},
		[]int{60, 90},
	)
	if err != nil {
		t.Fatalf("não esperava erro ao criar carona de teste: %v", err)
	}

	return carona
}

func TestCatalogoAdicionaCarona(t *testing.T) {
	catalogo := NovoCatalogoCaronas()
	carona := novaCaronaDeTeste(t, "carona-1")

	err := catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	caronaEncontrada, encontrada := catalogo.BuscarPorID("carona-1")
	if !encontrada {
		t.Fatal("esperava encontrar a carona adicionada")
	}

	if caronaEncontrada != carona {
		t.Error("a carona encontrada não é a mesma que foi adicionada")
	}
}

func TestCatalogoRejeitaIDDuplicado(t *testing.T) {
	catalogo := NovoCatalogoCaronas()

	err := catalogo.Adicionar(novaCaronaDeTeste(t, "carona-1"))
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar a primeira carona: %v", err)
	}

	err2 := catalogo.Adicionar(novaCaronaDeTeste(t, "carona-1"))
	if err2 == nil {
		t.Error("esperava erro ao adicionar uma carona com ID duplicado")
	}
}

func TestCatalogoBuscaCaronaPorIDInexistente(t *testing.T) {
	catalogo := NovoCatalogoCaronas()

	carona, encontrada := catalogo.BuscarPorID("carona-inexistente")

	if encontrada {
		t.Error("não esperava encontrar uma carona inexistente")
	}

	if carona != nil {
		t.Error("esperava nil para uma carona inexistente")
	}
}

func TestCatalogoListaTrechosDasCaronas(t *testing.T) {
	catalogo := NovoCatalogoCaronas()

	err := catalogo.Adicionar(novaCaronaDeTeste(t, "carona-1"))
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona-1: %v", err)
	}

	err = catalogo.Adicionar(novaCaronaDeTeste(t, "carona-2"))
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona-2: %v", err)
	}

	trechos := catalogo.ListarTrechos()
	if len(trechos) != 2 {
		t.Fatalf("quantidade de trechos = %d; esperava 2", len(trechos))
	}

	caronasEncontradas := make(map[string]bool)
	for _, trecho := range trechos {
		caronasEncontradas[trecho.CaronaID] = true
	}

	if !caronasEncontradas["carona-1"] {
		t.Error("esperava um trecho da carona-1")
	}

	if !caronasEncontradas["carona-2"] {
		t.Error("esperava um trecho da carona-2")
	}
}

func TestCatalogoConfirmaReservaEmTodosOsTrechos(t *testing.T) {
	// Uma viagem que passa por três cidades precisa ocupar seus dois trechos,
	// demonstrando que disponibilidade não é controlada pela carona inteira.
	catalogo := NovoCatalogoCaronas()
	carona := novaCaronaComRotaDeTeste(t, "carona-1", 4)

	err := catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	referencias := []domain.ReferenciaTrecho{
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0},
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 1},
	}

	reserva, err := catalogo.ConfirmarReserva("reserva-1", "passageiro-1", 2, referencias)
	if err != nil {
		t.Fatalf("não esperava erro ao confirmar reserva: %v", err)
	}

	if !reserva.EstaConfirmada() {
		t.Error("esperava reserva confirmada")
	}

	trechos := carona.Trechos()
	if trechos[0].AssentosDisponiveis != 2 || trechos[1].AssentosDisponiveis != 2 {
		t.Errorf(
			"assentos disponíveis = %d e %d; esperava 2 e 2",
			trechos[0].AssentosDisponiveis,
			trechos[1].AssentosDisponiveis,
		)
	}

	reservaEncontrada, encontrada := catalogo.BuscarReservaPorID("reserva-1")
	if !encontrada || reservaEncontrada != reserva {
		t.Error("esperava encontrar a reserva confirmada no catálogo")
	}
}

func TestCatalogoNaoAlteraNenhumTrechoQuandoReservaFalha(t *testing.T) {
	// Esta é a propriedade de atomicidade: se um dos trechos falhar, nenhum dos
	// outros pode perder assentos por causa de uma reserva incompleta.
	catalogo := NovoCatalogoCaronas()
	primeiraCarona := novaCaronaComRotaDeTeste(t, "carona-1", 6)
	segundaCarona := novaCaronaDeTeste(t, "carona-2")

	err := catalogo.Adicionar(primeiraCarona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar primeira carona: %v", err)
	}

	err = catalogo.Adicionar(segundaCarona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar segunda carona: %v", err)
	}

	referencias := []domain.ReferenciaTrecho{
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0},
		{EmpresaID: "empresa-1", CaronaID: "carona-2", Ordem: 0},
	}

	_, err = catalogo.ConfirmarReserva("reserva-1", "passageiro-1", 5, referencias)
	if err == nil {
		t.Fatal("esperava erro ao reservar mais assentos do que o disponível")
	}

	trechosPrimeiraCarona := primeiraCarona.Trechos()
	trechosSegundaCarona := segundaCarona.Trechos()

	if trechosPrimeiraCarona[0].AssentosDisponiveis != 6 {
		t.Errorf(
			"assentos da primeira carona = %d; esperava 6",
			trechosPrimeiraCarona[0].AssentosDisponiveis,
		)
	}

	if trechosSegundaCarona[0].AssentosDisponiveis != 4 {
		t.Errorf(
			"assentos da segunda carona = %d; esperava 4",
			trechosSegundaCarona[0].AssentosDisponiveis,
		)
	}
}

func TestCatalogoCancelaReservaEDevolveAssentos(t *testing.T) {
	// O cancelamento deve devolver exatamente a quantidade ocupada em todos os
	// trechos da reserva e trocar seu status para cancelada.
	catalogo := NovoCatalogoCaronas()
	carona := novaCaronaComRotaDeTeste(t, "carona-1", 4)

	err := catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	referencias := []domain.ReferenciaTrecho{
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0},
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 1},
	}

	_, err = catalogo.ConfirmarReserva("reserva-1", "passageiro-1", 2, referencias)
	if err != nil {
		t.Fatalf("não esperava erro ao confirmar reserva: %v", err)
	}

	err = catalogo.CancelarReserva("reserva-1", "passageiro-1")
	if err != nil {
		t.Fatalf("não esperava erro ao cancelar reserva: %v", err)
	}

	trechos := carona.Trechos()
	if trechos[0].AssentosDisponiveis != 4 || trechos[1].AssentosDisponiveis != 4 {
		t.Errorf(
			"assentos disponíveis = %d e %d; esperava 4 e 4",
			trechos[0].AssentosDisponiveis,
			trechos[1].AssentosDisponiveis,
		)
	}

	reserva, encontrada := catalogo.BuscarReservaPorID("reserva-1")
	if !encontrada {
		t.Fatal("esperava encontrar a reserva cancelada")
	}

	if reserva.Status != domain.StatusReservaCancelada {
		t.Errorf("status = %s; esperava cancelada", reserva.Status)
	}
}

func TestCatalogoImpedeCancelamentoPorOutroPassageiro(t *testing.T) {
	catalogo := NovoCatalogoCaronas()
	carona := novaCaronaDeTeste(t, "carona-1")

	err := catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	referencias := []domain.ReferenciaTrecho{{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0}}
	_, err = catalogo.ConfirmarReserva("reserva-1", "passageiro-1", 2, referencias)
	if err != nil {
		t.Fatalf("não esperava erro ao confirmar reserva: %v", err)
	}

	err = catalogo.CancelarReserva("reserva-1", "passageiro-2")
	if err == nil {
		t.Fatal("esperava erro ao cancelar reserva de outro passageiro")
	}

	trechos := carona.Trechos()
	if trechos[0].AssentosDisponiveis != 2 {
		t.Errorf(
			"assentos disponíveis = %d; esperava 2",
			trechos[0].AssentosDisponiveis,
		)
	}
}

func TestCatalogoImpedeCancelarReservaDuasVezes(t *testing.T) {
	catalogo := NovoCatalogoCaronas()
	carona := novaCaronaDeTeste(t, "carona-1")

	err := catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	referencias := []domain.ReferenciaTrecho{{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0}}
	_, err = catalogo.ConfirmarReserva("reserva-1", "passageiro-1", 2, referencias)
	if err != nil {
		t.Fatalf("não esperava erro ao confirmar reserva: %v", err)
	}

	err = catalogo.CancelarReserva("reserva-1", "passageiro-1")
	if err != nil {
		t.Fatalf("não esperava erro no primeiro cancelamento: %v", err)
	}

	err = catalogo.CancelarReserva("reserva-1", "passageiro-1")
	if err == nil {
		t.Error("esperava erro no segundo cancelamento")
	}

	trechos := carona.Trechos()
	if trechos[0].AssentosDisponiveis != 4 {
		t.Errorf(
			"assentos disponíveis = %d; esperava 4",
			trechos[0].AssentosDisponiveis,
		)
	}
}

func TestCatalogoCancelaCaronaECancelaReservasAfetadasPorInteiro(t *testing.T) {
	// A reserva usa duas caronas. Cancelar a primeira deve cancelar a reserva
	// inteira e devolver assentos também na segunda, evitando itinerário parcial.
	catalogo := NovoCatalogoCaronas()
	primeiraCarona := novaCaronaDeTeste(t, "carona-1")
	segundaCarona := novaCaronaDeTeste(t, "carona-2")

	err := catalogo.Adicionar(primeiraCarona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar a primeira carona: %v", err)
	}
	err = catalogo.Adicionar(segundaCarona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar a segunda carona: %v", err)
	}

	_, err = catalogo.ConfirmarReserva(
		"reserva-1",
		"passageiro-1",
		2,
		[]domain.ReferenciaTrecho{
			{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0},
			{EmpresaID: "empresa-1", CaronaID: "carona-2", Ordem: 0},
		},
	)
	if err != nil {
		t.Fatalf("não esperava erro ao confirmar reserva: %v", err)
	}

	err = catalogo.CancelarCarona("carona-1", "motorista-1")
	if err != nil {
		t.Fatalf("não esperava erro ao cancelar a carona: %v", err)
	}

	if primeiraCarona.EstaAtiva() {
		t.Error("esperava que a primeira carona estivesse cancelada")
	}
	if len(catalogo.ListarTrechos()) != 1 {
		t.Error("a carona cancelada não deve mais aparecer para busca")
	}

	for _, carona := range []*domain.Carona{primeiraCarona, segundaCarona} {
		trechos := carona.Trechos()
		if trechos[0].AssentosDisponiveis != 4 {
			t.Errorf("assentos da %s = %d; esperava 4", carona.ID, trechos[0].AssentosDisponiveis)
		}
	}

	reserva, encontrada := catalogo.BuscarReservaPorID("reserva-1")
	if !encontrada || reserva.EstaConfirmada() {
		t.Error("a reserva afetada deveria estar cancelada por inteiro")
	}
}

func TestCatalogoImpedeOutroMotoristaDeCancelarCarona(t *testing.T) {
	catalogo := NovoCatalogoCaronas()
	carona := novaCaronaDeTeste(t, "carona-1")

	err := catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	err = catalogo.CancelarCarona("carona-1", "motorista-2")
	if err == nil {
		t.Fatal("esperava erro ao cancelar carona de outro motorista")
	}
	if !carona.EstaAtiva() {
		t.Error("a carona deveria permanecer ativa")
	}
}

func TestCatalogoImpedeOverbookingComPassageirosConcorrentes(t *testing.T) {
	// Vinte goroutines disputarão cinco vagas. O canal "inicio" funciona como
	// barreira para que elas tentem reservar quase no mesmo instante.
	catalogo := NovoCatalogoCaronas()
	carona := novaCaronaComRotaDeTeste(t, "carona-concorrente", 5)

	err := catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	const quantidadePassageiros = 20
	inicio := make(chan struct{})
	resultados := make(chan error, quantidadePassageiros)
	var grupo sync.WaitGroup

	for i := 0; i < quantidadePassageiros; i++ {
		grupo.Add(1)

		go func(numero int) {
			defer grupo.Done()
			<-inicio

			_, erroReserva := catalogo.ConfirmarReserva(
				fmt.Sprintf("reserva-%d", numero),
				fmt.Sprintf("passageiro-%d", numero),
				1,
				[]domain.ReferenciaTrecho{{EmpresaID: "empresa-1", CaronaID: "carona-concorrente", Ordem: 0}},
			)
			resultados <- erroReserva
		}(i)
	}

	inicioMedicao := time.Now()
	close(inicio) // Libera todas as goroutines que estavam em <-inicio.
	grupo.Wait()
	duracao := time.Since(inicioMedicao)
	close(resultados)

	sucessos := 0
	for erroReserva := range resultados {
		if erroReserva == nil {
			sucessos++
		}
	}

	if sucessos != 5 {
		t.Errorf("reservas confirmadas = %d; esperava 5", sucessos)
	}

	trechos := carona.Trechos()
	if trechos[0].AssentosDisponiveis != 0 {
		t.Errorf(
			"assentos disponíveis = %d; esperava 0",
			trechos[0].AssentosDisponiveis,
		)
	}

	// Além da disponibilidade, conferir o map garante que só cinco reservas
	// foram efetivamente registradas como confirmadas.
	if len(catalogo.reservas) != 5 {
		t.Errorf("reservas armazenadas = %d; esperava 5", len(catalogo.reservas))
	}

	t.Logf(
		"carga: %d passageiros, %d sucessos, %d falhas, duração %s",
		quantidadePassageiros,
		sucessos,
		quantidadePassageiros-sucessos,
		duracao,
	)
}
