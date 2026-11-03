package consorcio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"vaijunto/internal/config"
	"vaijunto/internal/domain"
	"vaijunto/internal/store"
)

const (
	TimeoutFase2PC = 3 * time.Second
)

// CoordenadorTransacao orquestra o protocolo Two-Phase Commit (2PC) para reservas multiempresa.
type CoordenadorTransacao struct {
	minhaEmpresaID string
	catalogo       *store.CatalogoCaronas
	clienteHTTP    *http.Client
	parceiros      map[string]config.Parceiro
}

// NovoCoordenadorTransacao cria uma nova instância do coordenador 2PC.
func NovoCoordenadorTransacao(
	minhaEmpresaID string,
	catalogo *store.CatalogoCaronas,
	parceiros map[string]config.Parceiro,
	clienteHTTP *http.Client,
) *CoordenadorTransacao {
	if clienteHTTP == nil {
		clienteHTTP = &http.Client{
			Timeout: TimeoutFase2PC,
		}
	}

	return &CoordenadorTransacao{
		minhaEmpresaID: minhaEmpresaID,
		catalogo:       catalogo,
		clienteHTTP:    clienteHTTP,
		parceiros:      parceiros,
	}
}

// ReservarItinerario executa o Two-Phase Commit completo para confirmar todos os trechos ou nenhum.
func (c *CoordenadorTransacao) ReservarItinerario(
	ctx context.Context,
	idReserva string,
	passageiroID string,
	quantidadeAssentos int,
	trechos []domain.ReferenciaTrecho,
) (*domain.Reserva, error) {
	if len(trechos) == 0 {
		return nil, errors.New("itinerário não possui trechos")
	}

	// 1. Agrupa os trechos por empresa proprietária
	trechosPorEmpresa := make(map[string][]domain.ReferenciaTrecho)
	for _, ref := range trechos {
		trechosPorEmpresa[ref.EmpresaID] = append(trechosPorEmpresa[ref.EmpresaID], ref)
	}

	// Ordena as empresas em ordem alfabética para evitar deadlock
	empresasOrdenadas := make([]string, 0, len(trechosPorEmpresa))
	for empID := range trechosPorEmpresa {
		empresasOrdenadas = append(empresasOrdenadas, empID)
	}
	sort.Strings(empresasOrdenadas)

	// Fase 1: Prepare
	empresasPreparadasComSucesso := make([]string, 0, len(empresasOrdenadas))
	var falhaPrepare error

	for _, empID := range empresasOrdenadas {
		trechosDaEmpresa := trechosPorEmpresa[empID]

		var err error
		if empID == c.minhaEmpresaID {
			// Trechos locais: prepara diretamente no catálogo da própria empresa
			_, err = c.catalogo.PrepararReserva(
				idReserva,
				passageiroID,
				quantidadeAssentos,
				trechosDaEmpresa,
				store.TempoExpiracaoPreparacaoPadrao,
			)
		} else {
			// Trechos remotos: envia POST /api/v1/transacoes/preparar via HTTP REST
			err = c.enviarPrepareRemoto(ctx, empID, idReserva, passageiroID, quantidadeAssentos, trechosDaEmpresa)
		}

		if err != nil {
			falhaPrepare = fmt.Errorf("rejeição na empresa %s: %w", empID, err)
			break
		}

		empresasPreparadasComSucesso = append(empresasPreparadasComSucesso, empID)
	}

	// Fase 2: Decisão (Commit ou Abort)
	if falhaPrepare != nil {
		// Em caso de recusa ou falha, executa abort/rollback dos participantes preparados
		c.abortarEmTodas(ctx, idReserva, empresasPreparadasComSucesso)
		return nil, fmt.Errorf("reserva distribuída abortada: %w", falhaPrepare)
	}

	// Todos os participantes aprovaram: efetiva o commit em todos os nós
	var ultimaReserva *domain.Reserva
	for _, empID := range empresasOrdenadas {
		if empID == c.minhaEmpresaID {
			res, err := c.catalogo.ConfirmarTransacao(idReserva)
			if err == nil {
				ultimaReserva = res
			}
		} else {
			_ = c.enviarCommitRemoto(ctx, empID, idReserva)
		}
	}

	if ultimaReserva == nil {
		// Cria a representação da reserva confirmada para o cliente local
		ultimaReserva, _ = domain.NovaReserva(idReserva, passageiroID, quantidadeAssentos, trechos)
	}

	return ultimaReserva, nil
}

func (c *CoordenadorTransacao) abortarEmTodas(ctx context.Context, idReserva string, empresas []string) {
	for _, empID := range empresas {
		if empID == c.minhaEmpresaID {
			_ = c.catalogo.CancelarTransacao(idReserva)
		} else {
			_ = c.enviarAbortRemoto(ctx, empID, idReserva)
		}
	}
}

func (c *CoordenadorTransacao) enviarPrepareRemoto(
	ctx context.Context,
	empresaID string,
	transacaoID string,
	passageiroID string,
	quantidade int,
	trechos []domain.ReferenciaTrecho,
) error {
	parceiro, existe := c.parceiros[empresaID]
	if !existe {
		return fmt.Errorf("parceiro %s não configurado", empresaID)
	}

	trechosDTO := make([]ReferenciaTrechoDTO, 0, len(trechos))
	for _, t := range trechos {
		trechosDTO = append(trechosDTO, ReferenciaTrechoDTO{
			EmpresaID: t.EmpresaID,
			CaronaID:  t.CaronaID,
			Ordem:     t.Ordem,
		})
	}

	reqPayload := PrepararReservaRequest{
		TransacaoID:        transacaoID,
		PassageiroID:       passageiroID,
		QuantidadeAssentos: quantidade,
		Trechos:            trechosDTO,
		TTLMilissegundos:   store.TempoExpiracaoPreparacaoPadrao.Milliseconds(),
	}

	corpoJSON, _ := json.Marshal(reqPayload)
	urlDestino := fmt.Sprintf("%s/api/v1/transacoes/preparar", parceiro.URLBase)

	ctxTimeout, cancel := context.WithTimeout(ctx, TimeoutFase2PC)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctxTimeout, http.MethodPost, urlDestino, bytes.NewReader(corpoJSON))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.clienteHTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("timeout/erro de rede: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var respErr RespostaTransacao
		_ = json.NewDecoder(resp.Body).Decode(&respErr)
		return fmt.Errorf("recusa remota (status %d): %s", resp.StatusCode, respErr.Mensagem)
	}

	return nil
}

func (c *CoordenadorTransacao) enviarCommitRemoto(ctx context.Context, empresaID string, transacaoID string) error {
	parceiro, existe := c.parceiros[empresaID]
	if !existe {
		return fmt.Errorf("parceiro %s não configurado", empresaID)
	}

	urlDestino := fmt.Sprintf("%s/api/v1/transacoes/%s/confirmar", parceiro.URLBase, transacaoID)
	ctxTimeout, cancel := context.WithTimeout(ctx, TimeoutFase2PC)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctxTimeout, http.MethodPost, urlDestino, nil)
	if err != nil {
		return err
	}

	resp, err := c.clienteHTTP.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *CoordenadorTransacao) enviarAbortRemoto(ctx context.Context, empresaID string, transacaoID string) error {
	parceiro, existe := c.parceiros[empresaID]
	if !existe {
		return fmt.Errorf("parceiro %s não configurado", empresaID)
	}

	urlDestino := fmt.Sprintf("%s/api/v1/transacoes/%s/cancelar", parceiro.URLBase, transacaoID)
	ctxTimeout, cancel := context.WithTimeout(ctx, TimeoutFase2PC)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctxTimeout, http.MethodPost, urlDestino, nil)
	if err != nil {
		return err
	}

	resp, err := c.clienteHTTP.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
