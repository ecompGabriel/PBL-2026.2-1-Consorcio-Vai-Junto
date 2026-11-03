package domain

import (
	"errors"
	"time"
)

type Trecho struct {
	// EmpresaID identifica a empresa detentora da carona e deste trecho no consórcio.
	EmpresaID           string
	// Ordem diferencia os trechos da mesma carona: uma rota A-B-C possui os
	// trechos 0 (A-B) e 1 (B-C), cada qual com vagas independentes.
	CaronaID            string
	Ordem               int
	Origem              string
	Destino             string
	Capacidade          int
	AssentosDisponiveis int
	PrecoCentavos       int64
	HorarioSaida        time.Time
	HorarioChegada      time.Time
}

func (t Trecho) TemVagas(quantidadeSolicitada int) bool {
	// Consultar vagas não altera o estado; a confirmação real acontece depois,
	// sob o mutex do catálogo compartilhado.
	if quantidadeSolicitada <= 0 {
		return false
	}

	return t.AssentosDisponiveis >= quantidadeSolicitada
}

func (t *Trecho) Reservar(quantidadeSolicitada int) error {
	// A disponibilidade nunca pode ficar negativa após uma reserva válida.
	if quantidadeSolicitada <= 0 {
		return errors.New("Pedido inválido")
	}
	if quantidadeSolicitada > t.AssentosDisponiveis {
		return errors.New("Quantidade solicitada maior que a quantidade de assentos disponíveis")
	}
	t.AssentosDisponiveis -= quantidadeSolicitada

	return nil
}

func (t *Trecho) CancelarReserva(quantidadeCancelada int) error {
	// Cancelar devolve assentos, mas nunca pode ultrapassar a capacidade inicial.
	if quantidadeCancelada <= 0 {
		return errors.New("Cancelamento inválido")
	}
	if t.AssentosDisponiveis+quantidadeCancelada > t.Capacidade {
		return errors.New("Cancelamento ultrapassa a capacidade do trecho")
	}
	t.AssentosDisponiveis += quantidadeCancelada
	return nil
}
