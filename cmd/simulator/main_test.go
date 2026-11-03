package main

import (
	"testing"
)

func TestSimuladorComprovandoAusenciaDeOverbooking(t *testing.T) {
	// Cenário com 30 passageiros disputando exatamente 5 vagas em um itinerário multiempresa
	metricas := ExecutarCenarioCarga(30, nil)

	if metricas.Sucessos != 5 {
		t.Fatalf("Esperava exatamente 5 sucessos (capacidade máxima), mas obteve %d (overbooking ou erro de reserva)", metricas.Sucessos)
	}

	if metricas.FalhasOuAbortos != 25 {
		t.Fatalf("Esperava 25 falhas ordenadas por falta de vagas, mas obteve %d", metricas.FalhasOuAbortos)
	}

	t.Logf("Teste sob carga concluído com sucesso: %d sucessos, %d abortos seguros em %v",
		metricas.Sucessos, metricas.FalhasOuAbortos, metricas.TempoTotal)
}

func TestSimuladorComInjecaoDeFalhas(t *testing.T) {
	// Cenário com passageiros concorrentes e injeção de falhas
	falhasInjetadas := 0
	metricas := ExecutarCenarioCarga(20, func(empresaID string) {
		falhasInjetadas++
	})

	// Garante que o total de requisições foi contabilizado e o sistema não travou em deadlock
	if metricas.TotalRequisicoes != 20 {
		t.Errorf("Total de requisições esperado 20, obteve %d", metricas.TotalRequisicoes)
	}

	// Não pode haver mais de 5 sucessos sob nenhuma hipótese
	if metricas.Sucessos > 5 {
		t.Fatalf("Overbooking detectado durante falhas! Sucessos: %d", metricas.Sucessos)
	}

	t.Logf("Simulação com falhas: %d quedas de rede injetadas na Empresa B! %d confirmados, %d abortados com segurança.",
		falhasInjetadas, metricas.Sucessos, metricas.FalhasOuAbortos)
}
