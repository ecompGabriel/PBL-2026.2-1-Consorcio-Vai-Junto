package consorcio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"vaijunto/internal/config"
	"vaijunto/internal/domain"
)

const (
	// Tempo limite padrão para consultar trechos em uma parceira
	TimeoutConsultaTrechos = 2 * time.Second
)

// ClienteConsorcio é o cliente HTTP usado para se comunicar com as outras empresas do consórcio.
type ClienteConsorcio struct {
	httpCliente *http.Client
	parceiros   map[string]config.Parceiro
}

// NovoCliente cria uma instância do cliente HTTP configurado com as empresas parceiras.
func NovoCliente(parceiros map[string]config.Parceiro, httpCliente *http.Client) *ClienteConsorcio {
	if httpCliente == nil {
		httpCliente = &http.Client{
			Timeout: TimeoutConsultaTrechos,
		}
	}

	return &ClienteConsorcio{
		httpCliente: httpCliente,
		parceiros:   parceiros,
	}
}

// BuscarTrechosEmTodasParceiras faz requisições GET /api/v1/trechos para todas as parceiras cadastradas.
// Se uma parceira estiver caída ou der timeout, ela é ignorada com segurança, sem derrubar a busca.
func (c *ClienteConsorcio) BuscarTrechosEmTodasParceiras(ctx context.Context) ([]domain.Trecho, map[string]error) {
	todosTrechos := make([]domain.Trecho, 0)
	errosPorParceiro := make(map[string]error)

	for id, parceiro := range c.parceiros {
		trechos, err := c.buscarTrechosDeUmParceiro(ctx, parceiro)
		if err != nil {
			// Registra a falha, mas NÃO interrompe a busca nos outros parceiros
			errosPorParceiro[id] = err
			continue
		}

		todosTrechos = append(todosTrechos, trechos...)
	}

	return todosTrechos, errosPorParceiro
}

// buscarTrechosDeUmParceiro faz a chamada HTTP GET para uma parceira específica com timeout.
func (c *ClienteConsorcio) buscarTrechosDeUmParceiro(ctx context.Context, parceiro config.Parceiro) ([]domain.Trecho, error) {
	urlCompleta := fmt.Sprintf("%s/api/v1/trechos", parceiro.URLBase)

	// Cria a requisição HTTP com cancelamento por Context e Timeout
	ctxComTimeout, cancelar := context.WithTimeout(ctx, TimeoutConsultaTrechos)
	defer cancelar()

	req, err := http.NewRequestWithContext(ctxComTimeout, http.MethodGet, urlCompleta, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição para %s: %w", parceiro.ID, err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpCliente.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar com parceiro %s: %w", parceiro.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("parceiro %s retornou status %d", parceiro.ID, resp.StatusCode)
	}

	var dtos []TrechoDTO
	err = json.NewDecoder(resp.Body).Decode(&dtos)
	if err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta do parceiro %s: %w", parceiro.ID, err)
	}

	trechos := make([]domain.Trecho, 0, len(dtos))
	for _, dto := range dtos {
		trecho, err := ConverterDTOParaTrecho(dto)
		if err != nil {
			// Se um trecho específico estiver corrompido, ignora apenas ele
			continue
		}
		trechos = append(trechos, trecho)
	}

	return trechos, nil
}
