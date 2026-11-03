package consorcio

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"vaijunto/internal/domain"
	"vaijunto/internal/store"
)

// ServidorConsorcio gerencia os endpoints da API REST oferecida às outras empresas.
type ServidorConsorcio struct {
	empresaID string
	catalogo  *store.CatalogoCaronas
	roteador  *http.ServeMux
}

// NovoServidor cria e registra as rotas da API REST do consórcio.
func NovoServidor(empresaID string, catalogo *store.CatalogoCaronas) *ServidorConsorcio {
	servidor := &ServidorConsorcio{
		empresaID: empresaID,
		catalogo:  catalogo,
		roteador:  http.NewServeMux(),
	}

	servidor.registrarRotas()

	return servidor
}

// Handler devolve o manipulador HTTP pronto para ser usado por um http.Server.
func (s *ServidorConsorcio) Handler() http.Handler {
	return s.roteador
}

// registrarRotas mapeia os endpoints da API REST.
func (s *ServidorConsorcio) registrarRotas() {
	// Endpoint de busca de trechos
	s.roteador.HandleFunc("/api/v1/trechos", s.tratarListarTrechos)

	// Endpoints da transação 2PC
	s.roteador.HandleFunc("/api/v1/transacoes/preparar", s.tratarPreparar)
	s.roteador.HandleFunc("/api/v1/transacoes/", s.tratarTransacaoAcao)
}

// tratarListarTrechos responde ao GET /api/v1/trechos com todos os trechos locais ativos.
func (s *ServidorConsorcio) tratarListarTrechos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}

	trechosLocais := s.catalogo.ListarTrechos()
	dtos := make([]TrechoDTO, 0, len(trechosLocais))
	for _, t := range trechosLocais {
		dtos = append(dtos, ConverterParaDTO(t))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(dtos)
}

// tratarPreparar atende ao POST /api/v1/transacoes/preparar (Fase 1 do 2PC)
func (s *ServidorConsorcio) tratarPreparar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req PrepararReservaRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RespostaTransacao{Sucesso: false, Mensagem: "payload inválido"})
		return
	}

	referencias := make([]domain.ReferenciaTrecho, 0, len(req.Trechos))
	for _, t := range req.Trechos {
		referencias = append(referencias, domain.ReferenciaTrecho{
			EmpresaID: t.EmpresaID,
			CaronaID:  t.CaronaID,
			Ordem:     t.Ordem,
		})
	}

	ttl := time.Duration(req.TTLMilissegundos) * time.Millisecond
	_, err = s.catalogo.PrepararReserva(req.TransacaoID, req.PassageiroID, req.QuantidadeAssentos, referencias, ttl)
	if err != nil {
		// Retorna conflito caso não haja disponibilidade ou ocorra erro
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(RespostaTransacao{Sucesso: false, Mensagem: err.Error()})
		return
	}

	// Retorna confirmação de preparação com sucesso
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RespostaTransacao{Sucesso: true, Mensagem: "preparado com sucesso"})
}

// tratarTransacaoAcao atende /api/v1/transacoes/{id}/confirmar ou /api/v1/transacoes/{id}/cancelar
func (s *ServidorConsorcio) tratarTransacaoAcao(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}

	// Extrai ID e acao da URL: /api/v1/transacoes/{id}/{acao}
	partes := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(partes) != 5 {
		http.NotFound(w, r)
		return
	}

	idTransacao := partes[3]
	acao := partes[4]

	w.Header().Set("Content-Type", "application/json")

	switch acao {
	case "confirmar":
		_, err := s.catalogo.ConfirmarTransacao(idTransacao)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(RespostaTransacao{Sucesso: false, Mensagem: err.Error()})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(RespostaTransacao{Sucesso: true, Mensagem: "transação confirmada"})

	case "cancelar":
		_ = s.catalogo.CancelarTransacao(idTransacao)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(RespostaTransacao{Sucesso: true, Mensagem: "transação cancelada"})

	default:
		http.NotFound(w, r)
	}
}
