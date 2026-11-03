package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"vaijunto/internal/domain"
)

const (
	// TTL padrao para transacoes em preparacao (Lease)
	TempoExpiracaoPreparacaoPadrao = 5 * time.Second
)

type PassageiroNoTrecho struct {
	PassageiroID       string
	QuantidadeAssentos int
}

type SituacaoTrecho struct {
	Trecho      domain.Trecho
	Passageiros []PassageiroNoTrecho
}

type SituacaoCarona struct {
	ID        string
	Cancelada bool
	Trechos   []SituacaoTrecho
}

// ReservaPreparada registra uma reserva pendente na Fase 1 do 2PC com prazo de expiracao
type ReservaPreparada struct {
	Reserva   *domain.Reserva
	ExpiraEm  time.Time
	Cancelado bool
}

type CatalogoCaronas struct {
	// Mutex de leitura/escrita para proteger o mapa de caronas e reservas
	mu sync.RWMutex

	// Mutex por trecho/carona para evitar gargalo monolitico (exclusao mutua granular)
	trechoLocks   map[string]*sync.Mutex
	trechoLocksMu sync.Mutex

	caronas    map[string]*domain.Carona
	reservas   map[string]*domain.Reserva
	preparadas map[string]*ReservaPreparada

	// Caminho do arquivo de snapshot para persistência em disco
	arquivoPersistencia string
}

func NovoCatalogoCaronas() *CatalogoCaronas {
	return NovoCatalogoComPersistencia("")
}

func NovoCatalogoComPersistencia(caminhoArquivo string) *CatalogoCaronas {
	catalogo := &CatalogoCaronas{
		trechoLocks:         make(map[string]*sync.Mutex),
		caronas:             make(map[string]*domain.Carona),
		reservas:            make(map[string]*domain.Reserva),
		preparadas:          make(map[string]*ReservaPreparada),
		arquivoPersistencia: caminhoArquivo,
	}

	if caminhoArquivo != "" {
		_ = catalogo.carregarDoDisco()
	}

	// Goroutine em background para expirar reservas que ficaram presas na preparacao (TTL)
	go catalogo.limparPreparadasExpiradas()

	return catalogo
}

func (catalogo *CatalogoCaronas) obterLockTrecho(caronaID string, ordem int) *sync.Mutex {
	chave := fmt.Sprintf("%s:%d", caronaID, ordem)
	catalogo.trechoLocksMu.Lock()
	defer catalogo.trechoLocksMu.Unlock()

	lock, existe := catalogo.trechoLocks[chave]
	if !existe {
		lock = &sync.Mutex{}
		catalogo.trechoLocks[chave] = lock
	}
	return lock
}

func (catalogo *CatalogoCaronas) Adicionar(carona *domain.Carona) error {
	if carona == nil {
		return errors.New("a carona nao pode ser nula")
	}

	catalogo.mu.Lock()
	defer catalogo.mu.Unlock()

	if _, existe := catalogo.caronas[carona.ID]; existe {
		return errors.New("já existe uma carona com esse id")
	}

	catalogo.caronas[carona.ID] = carona
	_ = catalogo.salvarNoDisco()

	return nil
}

func (catalogo *CatalogoCaronas) BuscarPorID(id string) (*domain.Carona, bool) {
	catalogo.mu.RLock()
	defer catalogo.mu.RUnlock()

	carona, existe := catalogo.caronas[id]
	if !existe {
		return nil, false
	}
	return carona, true
}

func (catalogo *CatalogoCaronas) ListarTrechos() []domain.Trecho {
	catalogo.mu.RLock()
	defer catalogo.mu.RUnlock()

	trechos := make([]domain.Trecho, 0)
	for _, carona := range catalogo.caronas {
		if !carona.EstaAtiva() {
			continue
		}
		trechos = append(trechos, carona.Trechos()...)
	}
	return trechos
}

// PrepararReserva executa a Fase 1 do 2PC: bloqueia temporariamente a vaga com TTL
func (catalogo *CatalogoCaronas) PrepararReserva(
	idReserva string,
	passageiroID string,
	quantidadeAssentos int,
	referencias []domain.ReferenciaTrecho,
	ttl time.Duration,
) (*domain.Reserva, error) {
	if ttl <= 0 {
		ttl = TempoExpiracaoPreparacaoPadrao
	}

	reserva, err := domain.NovaReserva(idReserva, passageiroID, quantidadeAssentos, referencias)
	if err != nil {
		return nil, err
	}

	// Ordena referencias para evitar deadlock local
	referenciasOrdenadas := append([]domain.ReferenciaTrecho(nil), referencias...)
	sort.Slice(referenciasOrdenadas, func(i, j int) bool {
		if referenciasOrdenadas[i].CaronaID != referenciasOrdenadas[j].CaronaID {
			return referenciasOrdenadas[i].CaronaID < referenciasOrdenadas[j].CaronaID
		}
		return referenciasOrdenadas[i].Ordem < referenciasOrdenadas[j].Ordem
	})

	// Adquire travas granulares por trecho
	locks := make([]*sync.Mutex, 0, len(referenciasOrdenadas))
	for _, ref := range referenciasOrdenadas {
		locks = append(locks, catalogo.obterLockTrecho(ref.CaronaID, ref.Ordem))
	}
	for _, l := range locks {
		l.Lock()
	}
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
	}()

	catalogo.mu.Lock()
	defer catalogo.mu.Unlock()

	if _, existe := catalogo.reservas[idReserva]; existe {
		return nil, errors.New("já existe uma reserva com esse ID")
	}
	if _, existe := catalogo.preparadas[idReserva]; existe {
		return nil, errors.New("já existe uma transação preparada com esse ID")
	}

	// Valida disponibilidade em todos os trechos
	for _, ref := range referencias {
		carona, encontrada := catalogo.caronas[ref.CaronaID]
		if !encontrada {
			return nil, errors.New("carona da reserva não encontrada")
		}
		if !carona.EstaAtiva() {
			return nil, errors.New("uma carona da reserva foi cancelada")
		}
		if !carona.TemVagasNoTrecho(ref.Ordem, quantidadeAssentos) {
			return nil, errors.New("não há vagas suficientes em todos os trechos")
		}
	}

	// Bloqueia as vagas provisoriamente
	referenciasReservadas := make([]domain.ReferenciaTrecho, 0, len(referencias))
	for _, ref := range referencias {
		carona := catalogo.caronas[ref.CaronaID]
		err = carona.ReservarNoTrecho(ref.Ordem, quantidadeAssentos)
		if err != nil {
			for _, rReservada := range referenciasReservadas {
				c := catalogo.caronas[rReservada.CaronaID]
				c.CancelarNoTrecho(rReservada.Ordem, quantidadeAssentos)
			}
			return nil, errors.New("falha ao reservar trecho na preparação")
		}
		referenciasReservadas = append(referenciasReservadas, ref)
	}

	catalogo.preparadas[idReserva] = &ReservaPreparada{
		Reserva:  reserva,
		ExpiraEm: time.Now().Add(ttl),
	}

	return reserva, nil
}

// ConfirmarTransacao executa o COMMIT da Fase 2 do 2PC
func (catalogo *CatalogoCaronas) ConfirmarTransacao(idReserva string) (*domain.Reserva, error) {
	catalogo.mu.Lock()
	defer catalogo.mu.Unlock()

	// Idempotencia: se ja estiver confirmada, retorna sucesso diretamente
	if r, existe := catalogo.reservas[idReserva]; existe {
		return r, nil
	}

	prep, existe := catalogo.preparadas[idReserva]
	if !existe {
		return nil, errors.New("transação preparada não encontrada ou expirada")
	}
	if prep.Cancelado {
		return nil, errors.New("transação já foi cancelada")
	}

	reserva := prep.Reserva
	reserva.Status = domain.StatusReservaConfirmada
	catalogo.reservas[idReserva] = reserva
	delete(catalogo.preparadas, idReserva)

	_ = catalogo.salvarNoDisco()
	return reserva, nil
}

func pertenceACarona(ref domain.ReferenciaTrecho, carona *domain.Carona) bool {
	return carona != nil && ref.CaronaID == carona.ID && (ref.EmpresaID == "" || ref.EmpresaID == carona.EmpresaID())
}

// CancelarTransacao executa o cancelamento/abort de uma reserva (preparada ou confirmada) e devolve os assentos
func (catalogo *CatalogoCaronas) CancelarTransacao(idReserva string) error {
	catalogo.mu.Lock()
	defer catalogo.mu.Unlock()

	// 1. Se estiver na fase de preparacao (Fase 1 do 2PC)
	if prep, existe := catalogo.preparadas[idReserva]; existe {
		if !prep.Cancelado {
			prep.Cancelado = true
			reserva := prep.Reserva
			for _, ref := range reserva.Trechos() {
				if carona, ok := catalogo.caronas[ref.CaronaID]; ok && pertenceACarona(ref, carona) {
					_ = carona.CancelarNoTrecho(ref.Ordem, reserva.QuantidadeAssentos)
				}
			}
			delete(catalogo.preparadas, idReserva)
		}
		return nil
	}

	// 2. Se ja for uma reserva confirmada (cancelamento distribuido)
	if reserva, existe := catalogo.reservas[idReserva]; existe {
		if reserva.EstaConfirmada() {
			for _, ref := range reserva.Trechos() {
				if carona, ok := catalogo.caronas[ref.CaronaID]; ok && pertenceACarona(ref, carona) {
					_ = carona.CancelarNoTrecho(ref.Ordem, reserva.QuantidadeAssentos)
				}
			}
			_ = reserva.Cancelar()
			_ = catalogo.salvarNoDisco()
		}
		return nil
	}

	return nil
}

// ConfirmarReserva mantem compatibilidade direta com transacoes locais (1 fase atômica)
func (catalogo *CatalogoCaronas) ConfirmarReserva(
	idReserva string,
	passageiroID string,
	quantidadeAssentos int,
	referencias []domain.ReferenciaTrecho,
) (*domain.Reserva, error) {
	_, err := catalogo.PrepararReserva(idReserva, passageiroID, quantidadeAssentos, referencias, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return catalogo.ConfirmarTransacao(idReserva)
}

// SalvarReservaRegistrada armazena no catalogo da empresa coordenadora uma reserva confirmada
func (catalogo *CatalogoCaronas) SalvarReservaRegistrada(reserva *domain.Reserva) {
	catalogo.mu.Lock()
	defer catalogo.mu.Unlock()
	catalogo.reservas[reserva.ID] = reserva
	_ = catalogo.salvarNoDisco()
}

func (catalogo *CatalogoCaronas) BuscarReservaPorID(id string) (*domain.Reserva, bool) {
	catalogo.mu.RLock()
	defer catalogo.mu.RUnlock()

	reserva, existe := catalogo.reservas[id]
	if !existe {
		return nil, false
	}
	return reserva, true
}

func (catalogo *CatalogoCaronas) ListarReservasDoPassageiro(passageiroID string) []domain.Reserva {
	catalogo.mu.RLock()
	defer catalogo.mu.RUnlock()

	reservas := make([]domain.Reserva, 0)
	for _, reserva := range catalogo.reservas {
		if reserva.PassageiroID == passageiroID {
			reservas = append(reservas, *reserva)
		}
	}

	sort.Slice(reservas, func(i, j int) bool {
		return reservas[i].ID < reservas[j].ID
	})
	return reservas
}

func (catalogo *CatalogoCaronas) ListarCaronasDoMotorista(motoristaID string) []SituacaoCarona {
	catalogo.mu.RLock()
	defer catalogo.mu.RUnlock()

	caronas := make([]SituacaoCarona, 0)
	for _, carona := range catalogo.caronas {
		if carona.MotoristaID() != motoristaID {
			continue
		}

		situacao := SituacaoCarona{
			ID:        carona.ID,
			Cancelada: carona.EstaCancelada(),
			Trechos:   make([]SituacaoTrecho, 0),
		}

		for _, trecho := range carona.Trechos() {
			situacaoTrecho := SituacaoTrecho{
				Trecho:      trecho,
				Passageiros: make([]PassageiroNoTrecho, 0),
			}

			for _, reserva := range catalogo.reservas {
				if !reserva.EstaConfirmada() {
					continue
				}

				for _, ref := range reserva.Trechos() {
					if pertenceACarona(ref, carona) && ref.Ordem == trecho.Ordem {
						situacaoTrecho.Passageiros = append(
							situacaoTrecho.Passageiros,
							PassageiroNoTrecho{
								PassageiroID:       reserva.PassageiroID,
								QuantidadeAssentos: reserva.QuantidadeAssentos,
							},
						)
					}
				}
			}

			sort.Slice(situacaoTrecho.Passageiros, func(i, j int) bool {
				return situacaoTrecho.Passageiros[i].PassageiroID < situacaoTrecho.Passageiros[j].PassageiroID
			})
			situacao.Trechos = append(situacao.Trechos, situacaoTrecho)
		}

		caronas = append(caronas, situacao)
	}

	sort.Slice(caronas, func(i, j int) bool {
		return caronas[i].ID < caronas[j].ID
	})
	return caronas
}

func (catalogo *CatalogoCaronas) CancelarReserva(idReserva string, passageiroID string) error {
	catalogo.mu.Lock()
	defer catalogo.mu.Unlock()

	reserva, existe := catalogo.reservas[idReserva]
	if !existe {
		return errors.New("reserva não encontrada")
	}

	if reserva.PassageiroID != passageiroID {
		return errors.New("a reserva não pertence ao passageiro informado")
	}

	if !reserva.EstaConfirmada() {
		return errors.New("a reserva não está confirmada")
	}

	for _, ref := range reserva.Trechos() {
		carona, encontrada := catalogo.caronas[ref.CaronaID]
		if encontrada && pertenceACarona(ref, carona) {
			_ = carona.CancelarNoTrecho(ref.Ordem, reserva.QuantidadeAssentos)
		}
	}

	_ = reserva.Cancelar()
	_ = catalogo.salvarNoDisco()
	return nil
}

func (catalogo *CatalogoCaronas) CancelarCarona(idCarona string, motoristaID string) error {
	_, err := catalogo.CancelarCaronaComReservas(idCarona, motoristaID)
	return err
}

func (catalogo *CatalogoCaronas) CancelarCaronaComReservas(idCarona string, motoristaID string) ([]domain.Reserva, error) {
	catalogo.mu.Lock()
	defer catalogo.mu.Unlock()

	carona, existe := catalogo.caronas[idCarona]
	if !existe {
		return nil, errors.New("carona não encontrada")
	}
	if carona.MotoristaID() != motoristaID {
		return nil, errors.New("a carona não pertence ao motorista informado")
	}
	if !carona.EstaAtiva() {
		return nil, errors.New("a carona já está cancelada")
	}

	var reservasAfetadas []domain.Reserva
	for _, reserva := range catalogo.reservas {
		if !reserva.EstaConfirmada() {
			continue
		}

		afetada := false
		for _, ref := range reserva.Trechos() {
			if pertenceACarona(ref, carona) {
				afetada = true
				break
			}
		}
		if !afetada {
			continue
		}

		for _, ref := range reserva.Trechos() {
			if c, ok := catalogo.caronas[ref.CaronaID]; ok && pertenceACarona(ref, c) {
				_ = c.CancelarNoTrecho(ref.Ordem, reserva.QuantidadeAssentos)
			}
		}
		_ = reserva.Cancelar()
		reservasAfetadas = append(reservasAfetadas, *reserva)
	}

	err := carona.Cancelar()
	_ = catalogo.salvarNoDisco()
	return reservasAfetadas, err
}

func (catalogo *CatalogoCaronas) limparPreparadasExpiradas() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		catalogo.mu.Lock()
		agora := time.Now()
		for id, prep := range catalogo.preparadas {
			if agora.After(prep.ExpiraEm) {
				// Libera assentos de preparações cujo lease/TTL expirou
				for _, ref := range prep.Reserva.Trechos() {
					if c, ok := catalogo.caronas[ref.CaronaID]; ok {
						_ = c.CancelarNoTrecho(ref.Ordem, prep.Reserva.QuantidadeAssentos)
					}
				}
				delete(catalogo.preparadas, id)
			}
		}
		catalogo.mu.Unlock()
	}
}

// Rotinas de persistência e recuperação do catálogo em disco (JSON)
type snapshotEstado struct {
	Caronas  []*domain.Carona  `json:"caronas"`
	Reservas []*domain.Reserva `json:"reservas"`
}

func (catalogo *CatalogoCaronas) salvarNoDisco() error {
	if catalogo.arquivoPersistencia == "" {
		return nil
	}

	caronasSlice := make([]*domain.Carona, 0, len(catalogo.caronas))
	for _, c := range catalogo.caronas {
		caronasSlice = append(caronasSlice, c)
	}

	reservasSlice := make([]*domain.Reserva, 0, len(catalogo.reservas))
	for _, r := range catalogo.reservas {
		reservasSlice = append(reservasSlice, r)
	}

	dados, err := json.MarshalIndent(snapshotEstado{
		Caronas:  caronasSlice,
		Reservas: reservasSlice,
	}, "", "  ")
	if err != nil {
		return err
	}

	_ = os.MkdirAll(filepath.Dir(catalogo.arquivoPersistencia), 0755)
	return os.WriteFile(catalogo.arquivoPersistencia, dados, 0644)
}

func (catalogo *CatalogoCaronas) carregarDoDisco() error {
	if catalogo.arquivoPersistencia == "" {
		return nil
	}

	dados, err := os.ReadFile(catalogo.arquivoPersistencia)
	if err != nil {
		return err
	}

	var estado snapshotEstado
	err = json.Unmarshal(dados, &estado)
	if err != nil {
		return err
	}

	for _, c := range estado.Caronas {
		catalogo.caronas[c.ID] = c
	}
	for _, r := range estado.Reservas {
		catalogo.reservas[r.ID] = r
	}
	return nil
}
