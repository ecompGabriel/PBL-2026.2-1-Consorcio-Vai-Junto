package consorcio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vaijunto/internal/config"
)

// roundTripperMock permite interceptar requisições HTTP em memória sem abrir portas de rede locais
type roundTripperMock struct {
	handler http.Handler
}

func (m *roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	gravador := httptest.NewRecorder()
	m.handler.ServeHTTP(gravador, req)
	return gravador.Result(), nil
}

func TestClienteBuscarTrechosComSucesso(t *testing.T) {
	// 1. Simula um servidor remoto (ex: Empresa Salvador) respondendo com 1 trecho
	handlerMock := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/trechos" || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		dtos := []TrechoDTO{
			{
				EmpresaID:           "empresa-b",
				CaronaID:            "carona-salvador-1",
				Ordem:               0,
				Origem:              "Salvador",
				Destino:             "Conquista",
				Capacidade:          4,
				AssentosDisponiveis: 3,
				PrecoCentavos:       3500,
				HorarioSaida:        time.Now().Add(time.Hour).Format(time.RFC3339),
				HorarioChegada:      time.Now().Add(4 * time.Hour).Format(time.RFC3339),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(dtos)
	})

	httpClienteMock := &http.Client{
		Transport: &roundTripperMock{handler: handlerMock},
	}

	// 2. Configura o cliente para conhecer esse parceiro simulado
	parceiros := map[string]config.Parceiro{
		"empresa-b": {
			ID:      "empresa-b",
			URLBase: "http://empresa-b:3090",
		},
	}

	cliente := NovoCliente(parceiros, httpClienteMock)

	// 3. Executa a busca
	trechos, erros := cliente.BuscarTrechosEmTodasParceiras(context.Background())

	// 4. Valida os resultados
	if len(erros) != 0 {
		t.Fatalf("não esperava erros de parceiros, obteve: %v", erros)
	}

	if len(trechos) != 1 {
		t.Fatalf("quantidade de trechos = %d; esperava 1", len(trechos))
	}

	if trechos[0].EmpresaID != "empresa-b" || trechos[0].Origem != "Salvador" || trechos[0].Destino != "Conquista" {
		t.Errorf("trecho recebido incorreto: %+v", trechos[0])
	}
}

func TestClienteToleraServidorParceiroCaido(t *testing.T) {
	// Configura um parceiro com uma porta inexistente/fechada (simulando servidor desligado)
	parceiros := map[string]config.Parceiro{
		"conquista": {
			ID:      "conquista",
			URLBase: "http://127.0.0.1:59999", // Porta que ninguém está ouvindo
		},
	}

	cliente := NovoCliente(parceiros, nil)

	// A busca NÃO pode travar nem dar pânico; deve apenas registrar o erro do parceiro
	trechos, erros := cliente.BuscarTrechosEmTodasParceiras(context.Background())

	if len(trechos) != 0 {
		t.Errorf("esperava 0 trechos, obteve %d", len(trechos))
	}

	if _, existe := erros["conquista"]; !existe {
		t.Error("esperava registro de erro para o parceiro conquista que estava caído")
	}
}
