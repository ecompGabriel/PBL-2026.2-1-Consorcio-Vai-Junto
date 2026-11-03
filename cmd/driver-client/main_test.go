package main

import (
	"testing"
	"time"
)

func TestConverterReaisParaCentavos(t *testing.T) {
	testes := []struct {
		texto    string
		esperado int64
	}{
		{texto: "15,50", esperado: 1550},
		{texto: "20.00", esperado: 2000},
		{texto: "7", esperado: 700},
		{texto: "3,5", esperado: 350},
	}

	for _, teste := range testes {
		centavos, err := converterReaisParaCentavos(teste.texto)
		if err != nil {
			t.Fatalf("não esperava erro para %s: %v", teste.texto, err)
		}

		if centavos != teste.esperado {
			t.Errorf("%s = %d centavos; esperava %d", teste.texto, centavos, teste.esperado)
		}
	}
}

func TestConverterReaisParaCentavosRejeitaFormatoInvalido(t *testing.T) {
	entradasInvalidas := []string{"-1", "1,", "10,500", "abc"}

	for _, entrada := range entradasInvalidas {
		_, err := converterReaisParaCentavos(entrada)
		if err == nil {
			t.Errorf("esperava erro para o preço inválido %s", entrada)
		}
	}
}

func TestConverterDataHoraParaRFC3339(t *testing.T) {
	horario, err := converterDataHoraParaRFC3339("17/09/2026", "08:00")
	if err != nil {
		t.Fatalf("não esperava erro ao converter data e horário: %v", err)
	}

	if horario != "2026-09-17T08:00:00-03:00" {
		t.Errorf("horário = %s; esperava 2026-09-17T08:00:00-03:00", horario)
	}
}

func TestConverterDataHoraParaRFC3339RejeitaFormatoInvalido(t *testing.T) {
	_, err := converterDataHoraParaRFC3339("17-09-2026", "8h")
	if err == nil {
		t.Error("esperava erro para data e horário em formato inválido")
	}
}

func TestValidarHorarioFuturo(t *testing.T) {
	agora := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)

	err := validarHorarioFuturo("2026-09-15T13:00:00Z", agora)
	if err != nil {
		t.Fatalf("não esperava erro para horário futuro: %v", err)
	}

	err = validarHorarioFuturo("2026-09-15T11:00:00Z", agora)
	if err == nil {
		t.Error("esperava erro para horário no passado")
	}
}
