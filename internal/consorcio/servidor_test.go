package consorcio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vaijunto/internal/domain"
	"vaijunto/internal/store"
)

func TestServidorListarTrechosRetorna200EJSONValido(t *testing.T) {
	catalogo := store.NovoCatalogoCaronas()

	carona, err := domain.NovaCarona(
		"feira",
		"carona-1",
		"motorista-1",
		time.Date(2026, time.September, 17, 8, 0, 0, 0, time.UTC),
		[]string{"Feira", "Salvador"},
		4,
		[]int64{2000},
		[]int{60},
	)
	if err != nil {
		t.Fatalf("não esperava erro ao criar carona: %v", err)
	}

	err = catalogo.Adicionar(carona)
	if err != nil {
		t.Fatalf("não esperava erro ao adicionar carona: %v", err)
	}

	servidor := NovoServidor("feira", catalogo)

	requisicao := httptest.NewRequest(http.MethodGet, "/api/v1/trechos", nil)
	gravador := httptest.NewRecorder()

	servidor.Handler().ServeHTTP(gravador, requisicao)

	if gravador.Code != http.StatusOK {
		t.Fatalf("status = %d; esperava %d", gravador.Code, http.StatusOK)
	}

	tipoConteudo := gravador.Header().Get("Content-Type")
	if tipoConteudo != "application/json" {
		t.Errorf("Content-Type = %s; esperava application/json", tipoConteudo)
	}

	var trechosRecebidos []TrechoDTO
	err = json.Unmarshal(gravador.Body.Bytes(), &trechosRecebidos)
	if err != nil {
		t.Fatalf("erro ao decodificar JSON de resposta: %v", err)
	}

	if len(trechosRecebidos) != 1 {
		t.Fatalf("quantidade de trechos = %d; esperava 1", len(trechosRecebidos))
	}

	trecho := trechosRecebidos[0]
	if trecho.EmpresaID != "feira" || trecho.CaronaID != "carona-1" || trecho.Origem != "Feira" || trecho.Destino != "Salvador" {
		t.Errorf("dados do trecho incorretos: %+v", trecho)
	}

	trechoDominio, err := ConverterDTOParaTrecho(trecho)
	if err != nil {
		t.Fatalf("erro ao converter DTO de volta para Trecho de domínio: %v", err)
	}
	if trechoDominio.EmpresaID != "feira" || trechoDominio.Origem != "Feira" {
		t.Errorf("conversão para domínio falhou: %+v", trechoDominio)
	}
}

func TestServidorListarTrechosRejeitaMetodoNaoGet(t *testing.T) {
	catalogo := store.NovoCatalogoCaronas()
	servidor := NovoServidor("feira", catalogo)

	requisicao := httptest.NewRequest(http.MethodPost, "/api/v1/trechos", nil)
	gravador := httptest.NewRecorder()

	servidor.Handler().ServeHTTP(gravador, requisicao)

	if gravador.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; esperava %d (Method Not Allowed)", gravador.Code, http.StatusMethodNotAllowed)
	}
}
