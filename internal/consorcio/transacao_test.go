package consorcio

import (
	"context"
	"net/http"
	"testing"
	"time"

	"vaijunto/internal/config"
	"vaijunto/internal/domain"
	"vaijunto/internal/store"
)

func TestCoordenadorTransacao2PCComSucesso(t *testing.T) {
	// 1. Cria duas empresas simuladas: "empresa-a" (coordenadora) e "empresa-b" (participante)
	catalogoA := store.NovoCatalogoCaronas()
	catalogoB := store.NovoCatalogoCaronas()

	caronaA, _ := domain.NovaCarona(
		"empresa-a",
		"carona-a",
		"motorista-1",
		time.Now().Add(time.Hour),
		[]string{"Cidade1", "Cidade2"},
		4,
		[]int64{1500},
		[]int{60},
	)
	_ = catalogoA.Adicionar(caronaA)

	caronaB, _ := domain.NovaCarona(
		"empresa-b",
		"carona-b",
		"motorista-2",
		time.Now().Add(2*time.Hour),
		[]string{"Cidade2", "Cidade3"},
		4,
		[]int64{2500},
		[]int{90},
	)
	_ = catalogoB.Adicionar(caronaB)

	// Servidor REST da empresa-b
	servidorB := NovoServidor("empresa-b", catalogoB)
	httpClienteParaB := &http.Client{
		Transport: &roundTripperMock{handler: servidorB.Handler()},
	}

	parceiros := map[string]config.Parceiro{
		"empresa-b": {
			ID:      "empresa-b",
			URLBase: "http://empresa-b:3090",
		},
	}

	coordenador := NovoCoordenadorTransacao("empresa-a", catalogoA, parceiros, httpClienteParaB)

	// Itinerário combinando Trecho de A + Trecho de B
	trechosReserva := []domain.ReferenciaTrecho{
		{EmpresaID: "empresa-a", CaronaID: "carona-a", Ordem: 0},
		{EmpresaID: "empresa-b", CaronaID: "carona-b", Ordem: 0},
	}

	reserva, err := coordenador.ReservarItinerario(
		context.Background(),
		"reserva-tx-1",
		"passageiro-1",
		2,
		trechosReserva,
	)

	if err != nil {
		t.Fatalf("não esperava erro na reserva 2PC: %v", err)
	}
	if reserva == nil || !reserva.EstaConfirmada() {
		t.Fatal("esperava reserva confirmada com sucesso")
	}

	// Verifica se os assentos foram consumidos com sucesso em AMBAS as empresas
	trechosA := caronaA.Trechos()
	if trechosA[0].AssentosDisponiveis != 2 {
		t.Errorf("assentos em A = %d; esperava 2", trechosA[0].AssentosDisponiveis)
	}

	trechosB := caronaB.Trechos()
	if trechosB[0].AssentosDisponiveis != 2 {
		t.Errorf("assentos em B = %d; esperava 2", trechosB[0].AssentosDisponiveis)
	}
}

func TestCoordenadorTransacao2PCAbortQuandoParticipanteRecusa(t *testing.T) {
	// Se a empresa B não tiver assentos suficientes, a empresa A DEVE fazer rollback e liberar o seu trecho!
	catalogoA := store.NovoCatalogoCaronas()
	catalogoB := store.NovoCatalogoCaronas()

	caronaA, _ := domain.NovaCarona(
		"empresa-a",
		"carona-a",
		"motorista-1",
		time.Now().Add(time.Hour),
		[]string{"Cidade1", "Cidade2"},
		4,
		[]int64{1500},
		[]int{60},
	)
	_ = catalogoA.Adicionar(caronaA)

	// Empresa B só tem 1 vaga disponível
	caronaB, _ := domain.NovaCarona(
		"empresa-b",
		"carona-b",
		"motorista-2",
		time.Now().Add(2*time.Hour),
		[]string{"Cidade2", "Cidade3"},
		1,
		[]int64{2500},
		[]int{90},
	)
	_ = catalogoB.Adicionar(caronaB)

	servidorB := NovoServidor("empresa-b", catalogoB)
	httpClienteParaB := &http.Client{
		Transport: &roundTripperMock{handler: servidorB.Handler()},
	}

	parceiros := map[string]config.Parceiro{
		"empresa-b": {
			ID:      "empresa-b",
			URLBase: "http://empresa-b:3090",
		},
	}

	coordenador := NovoCoordenadorTransacao("empresa-a", catalogoA, parceiros, httpClienteParaB)

	// Passageiro pede 2 vagas (A tem 4, mas B só tem 1)
	trechosReserva := []domain.ReferenciaTrecho{
		{EmpresaID: "empresa-a", CaronaID: "carona-a", Ordem: 0},
		{EmpresaID: "empresa-b", CaronaID: "carona-b", Ordem: 0},
	}

	reserva, err := coordenador.ReservarItinerario(
		context.Background(),
		"reserva-tx-falha",
		"passageiro-1",
		2,
		trechosReserva,
	)

	if err == nil {
		t.Fatal("esperava erro de aborto na reserva sem vagas em B")
	}
	if reserva != nil {
		t.Fatal("não esperava reserva criada após aborto")
	}

	// Invariante de Atomicidade: os assentos na Empresa A DEVEM continuar 4 (nenhum assento preso!)
	trechosA := caronaA.Trechos()
	if trechosA[0].AssentosDisponiveis != 4 {
		t.Errorf("assentos em A após aborto = %d; esperava 4 (rollback não ocorreu)", trechosA[0].AssentosDisponiveis)
	}

	trechosB := caronaB.Trechos()
	if trechosB[0].AssentosDisponiveis != 1 {
		t.Errorf("assentos em B após aborto = %d; esperava 1", trechosB[0].AssentosDisponiveis)
	}
}
