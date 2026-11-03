package domain

import "testing"

func referenciasDeTrechosDeTeste() []ReferenciaTrecho {
	return []ReferenciaTrecho{
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0},
		{EmpresaID: "empresa-1", CaronaID: "carona-2", Ordem: 1},
	}
}

func TestNovaReservaCriaReservaConfirmada(t *testing.T) {
	trechos := referenciasDeTrechosDeTeste()

	reserva, err := NovaReserva("reserva-1", "passageiro-1", 2, trechos)
	if err != nil {
		t.Fatalf("não esperava erro ao criar reserva: %v", err)
	}

	if reserva.ID != "reserva-1" {
		t.Errorf("ID = %s; esperava reserva-1", reserva.ID)
	}

	if reserva.PassageiroID != "passageiro-1" {
		t.Errorf("passageiro = %s; esperava passageiro-1", reserva.PassageiroID)
	}

	if reserva.QuantidadeAssentos != 2 {
		t.Errorf("quantidade de assentos = %d; esperava 2", reserva.QuantidadeAssentos)
	}

	if !reserva.EstaConfirmada() {
		t.Error("esperava que a nova reserva estivesse confirmada")
	}
}

func TestReservaTrechosDevolveCopia(t *testing.T) {
	reserva, err := NovaReserva("reserva-1", "passageiro-1", 1, referenciasDeTrechosDeTeste())
	if err != nil {
		t.Fatalf("não esperava erro ao criar reserva: %v", err)
	}

	trechos := reserva.Trechos()
	trechos[0].CaronaID = "outra-carona"

	trechosOriginais := reserva.Trechos()
	if trechosOriginais[0].CaronaID == "outra-carona" {
		t.Error("alterar a cópia dos trechos não deveria alterar a reserva")
	}
}

func TestNovaReservaRejeitaDadosInvalidos(t *testing.T) {
	_, err := NovaReserva("", "passageiro-1", 1, referenciasDeTrechosDeTeste())
	if err == nil {
		t.Error("esperava erro para ID de reserva vazio")
	}

	_, err = NovaReserva("reserva-1", "", 1, referenciasDeTrechosDeTeste())
	if err == nil {
		t.Error("esperava erro para ID de passageiro vazio")
	}

	_, err = NovaReserva("reserva-1", "passageiro-1", 0, referenciasDeTrechosDeTeste())
	if err == nil {
		t.Error("esperava erro para quantidade de assentos igual a zero")
	}

	_, err = NovaReserva("reserva-1", "passageiro-1", 1, nil)
	if err == nil {
		t.Error("esperava erro para reserva sem trechos")
	}

	trechoRepetido := []ReferenciaTrecho{
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0},
		{EmpresaID: "empresa-1", CaronaID: "carona-1", Ordem: 0},
	}

	_, err = NovaReserva("reserva-1", "passageiro-1", 1, trechoRepetido)
	if err == nil {
		t.Error("esperava erro para trecho repetido na reserva")
	}

	trechoSemEmpresa := []ReferenciaTrecho{
		{EmpresaID: "", CaronaID: "carona-1", Ordem: 0},
	}
	_, err = NovaReserva("reserva-1", "passageiro-1", 1, trechoSemEmpresa)
	if err == nil {
		t.Error("esperava erro para trecho sem ID da empresa")
	}
}

func TestReservaCancelar(t *testing.T) {
	reserva, err := NovaReserva("reserva-1", "passageiro-1", 1, referenciasDeTrechosDeTeste())
	if err != nil {
		t.Fatalf("não esperava erro ao criar reserva: %v", err)
	}

	err = reserva.Cancelar()
	if err != nil {
		t.Fatalf("não esperava erro ao cancelar reserva: %v", err)
	}

	if reserva.Status != StatusReservaCancelada {
		t.Errorf("status = %s; esperava cancelada", reserva.Status)
	}

	err = reserva.Cancelar()
	if err == nil {
		t.Error("esperava erro ao cancelar uma reserva já cancelada")
	}
}
