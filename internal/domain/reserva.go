package domain

import (
	"errors"
	"strings"
)

type StatusReserva string

const (
	StatusReservaPreparada  StatusReserva = "preparada"
	StatusReservaConfirmada StatusReserva = "confirmada"
	StatusReservaCancelada  StatusReserva = "cancelada"
)

type ReferenciaTrecho struct {
	// EmpresaID + CaronaID + Ordem identifica unicamente um trecho dentro do consórcio.
	EmpresaID string
	CaronaID  string
	Ordem     int
}

type Reserva struct {
	ID                 string        // Chave usada pelo passageiro para consultar/cancelar.
	PassageiroID       string        // Dono, validado pela sessão no servidor.
	QuantidadeAssentos int           // Mesma quantidade ocupada em todos os trechos.
	Status             StatusReserva // Confirmada ocupa vagas; cancelada não ocupa.
	trechos            []ReferenciaTrecho
}

func NovaReserva(
	id string,
	passageiroID string,
	quantidadeAssentos int,
	trechos []ReferenciaTrecho,
) (*Reserva, error) {
	// A reserva nasce confirmada apenas no domínio; o catálogo só a armazena
	// depois de validar e alterar todos os trechos de forma atômica.
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("o ID da reserva é obrigatório")
	}

	if strings.TrimSpace(passageiroID) == "" {
		return nil, errors.New("o ID do passageiro é obrigatório")
	}

	if quantidadeAssentos <= 0 {
		return nil, errors.New("a quantidade de assentos deve ser positiva")
	}

	if len(trechos) == 0 {
		return nil, errors.New("a reserva precisa ter ao menos um trecho")
	}

	// Impede reservar duas vezes o mesmo trecho dentro da mesma reserva.
	referenciasVistas := make(map[ReferenciaTrecho]bool)

	for _, trecho := range trechos {
		if strings.TrimSpace(trecho.EmpresaID) == "" {
			return nil, errors.New("o ID da empresa do trecho é obrigatório")
		}

		if strings.TrimSpace(trecho.CaronaID) == "" {
			return nil, errors.New("o ID da carona do trecho é obrigatório")
		}

		if trecho.Ordem < 0 {
			return nil, errors.New("a ordem do trecho não pode ser negativa")
		}

		if referenciasVistas[trecho] {
			return nil, errors.New("um trecho não pode aparecer duas vezes na mesma reserva")
		}

		referenciasVistas[trecho] = true
	}

	return &Reserva{
		ID:                 id,
		PassageiroID:       passageiroID,
		QuantidadeAssentos: quantidadeAssentos,
		Status:             StatusReservaConfirmada,
		trechos:            append([]ReferenciaTrecho(nil), trechos...),
	}, nil
}

func (reserva Reserva) Trechos() []ReferenciaTrecho {
	// A cópia impede que clientes do domínio modifiquem a reserva internamente.
	return append([]ReferenciaTrecho(nil), reserva.trechos...)
}

func (reserva Reserva) EstaConfirmada() bool {
	// Centraliza a comparação de status para evitar espalhar strings pelo código.
	return reserva.Status == StatusReservaConfirmada
}

func (reserva Reserva) EstaPreparada() bool {
	return reserva.Status == StatusReservaPreparada
}

func (reserva *Reserva) Confirmar() error {
	if reserva.Status != StatusReservaPreparada {
		return errors.New("somente uma reserva preparada pode ser confirmada")
	}
	reserva.Status = StatusReservaConfirmada
	return nil
}

func (reserva *Reserva) Cancelar() error {
	// Impede que o mesmo ID devolva assentos duas vezes em cancelamentos repetidos.
	if reserva.Status == StatusReservaCancelada {
		return errors.New("a reserva já está cancelada")
	}

	reserva.Status = StatusReservaCancelada
	return nil
}
