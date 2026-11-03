package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"vaijunto/internal/config"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/domain"
	"vaijunto/internal/search"
	"vaijunto/internal/store"
)

type MetricasSimulacao struct {
	TotalRequisicoes int64
	Sucessos         int64
	FalhasOuAbortos  int64
	TempoTotal       time.Duration
}

func main() {
	numPassageiros := flag.Int("passageiros", 30, "Número de passageiros concorrentes")
	flag.Parse()

	log.Printf("Iniciando Simulador Autônomo com %d passageiros simultâneos...", *numPassageiros)

	metricas := ExecutarCenarioCarga(*numPassageiros, nil)

	fmt.Println("==================================================")
	fmt.Println("RELATÓRIO DO SIMULADOR DE CARGA (Problema 2)")
	fmt.Println("==================================================")
	fmt.Printf("Passageiros Simultâneos: %d\n", *numPassageiros)
	fmt.Printf("Total de Reservas Tentadas: %d\n", metricas.TotalRequisicoes)
	fmt.Printf("Reservas Confirmadas (Commit): %d\n", metricas.Sucessos)
	fmt.Printf("Reservas Abortadas / Sem Vagas: %d\n", metricas.FalhasOuAbortos)
	fmt.Printf("Tempo de Execução: %v\n", metricas.TempoTotal)
	fmt.Println("==================================================")
}

// ExecutarCenarioCarga emula três empresas independentes em memória com passageiros concorrentes
func ExecutarCenarioCarga(quantidadePassageiros int, falhaInjetadaFn func(empresaID string)) MetricasSimulacao {
	// 1. Cria os três servidores de empresas independentes
	catA := store.NovoCatalogoCaronas()
	catB := store.NovoCatalogoCaronas()
	catC := store.NovoCatalogoCaronas()

	// Publica caronas formando um itinerário conectado: Cidade1 -> Cidade2 -> Cidade3
	// Empresa A tem 5 vagas de Cidade1 para Cidade2
	caronaA, _ := domain.NovaCarona(
		"empresa-a",
		"carona-a1",
		"motorista-a",
		time.Now().Add(time.Hour),
		[]string{"Cidade1", "Cidade2"},
		5,
		[]int64{1000},
		[]int{60},
	)
	_ = catA.Adicionar(caronaA)

	// Empresa B tem 5 vagas de Cidade2 para Cidade3
	caronaB, _ := domain.NovaCarona(
		"empresa-b",
		"carona-b1",
		"motorista-b",
		time.Now().Add(3*time.Hour),
		[]string{"Cidade2", "Cidade3"},
		5,
		[]int64{1500},
		[]int{60},
	)
	_ = catB.Adicionar(caronaB)

	srvA := consorcio.NovoServidor("empresa-a", catA)
	srvB := consorcio.NovoServidor("empresa-b", catB)
	srvC := consorcio.NovoServidor("empresa-c", catC)

	roteador := &roundTripperRoteador{
		servidores: map[string]http.Handler{
			"http://empresa-a:3090": srvA.Handler(),
			"http://empresa-b:3090": srvB.Handler(),
			"http://empresa-c:3090": srvC.Handler(),
		},
	}

	clienteMapeado := &http.Client{
		Transport: roteador,
	}

	parceirosDeA := map[string]config.Parceiro{
		"empresa-b": {ID: "empresa-b", URLBase: "http://empresa-b:3090"},
		"empresa-c": {ID: "empresa-c", URLBase: "http://empresa-c:3090"},
	}

	coordA := consorcio.NovoCoordenadorTransacao("empresa-a", catA, parceirosDeA, clienteMapeado)
	clienteConsorcioA := consorcio.NovoCliente(parceirosDeA, clienteMapeado)

	// 2. Dispara a carga concorrente de passageiros
	var sucessos int64
	var falhas int64
	var wg sync.WaitGroup
	barreiraInicio := make(chan struct{})

	inicio := time.Now()

	for i := 0; i < quantidadePassageiros; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-barreiraInicio // Barreira para todas as goroutines partirem no mesmo instante

			// 1. Busca trechos locais + remotos
			remotos, _ := clienteConsorcioA.BuscarTrechosEmTodasParceiras(context.Background())
			todosTrechos := append(catA.ListarTrechos(), remotos...)

			// 2. Encontra itinerário via DFS
			itinerarios, err := search.BuscarItinerarios(todosTrechos, "Cidade1", "Cidade3", 1, time.Now())
			if err != nil || len(itinerarios) == 0 {
				atomic.AddInt64(&falhas, 1)
				return
			}

			// Injeção de falha aleatória se a função tiver sido fornecida
			if falhaInjetadaFn != nil && rand.Float32() < 0.2 {
				roteador.DesligarServidor("http://empresa-b:3090")
				falhaInjetadaFn("empresa-b")
			}

			// 3. Monta referências dos trechos
			refs := make([]domain.ReferenciaTrecho, 0, len(itinerarios[0].Trechos))
			for _, t := range itinerarios[0].Trechos {
				refs = append(refs, domain.ReferenciaTrecho{
					EmpresaID: t.EmpresaID,
					CaronaID:  t.CaronaID,
					Ordem:     t.Ordem,
				})
			}

			// 4. Executa 2PC
			idReserva := fmt.Sprintf("reserva-sim-%d", id)
			passageiroID := fmt.Sprintf("passageiro-%d", id)
			_, err = coordA.ReservarItinerario(context.Background(), idReserva, passageiroID, 1, refs)
			if err != nil {
				atomic.AddInt64(&falhas, 1)
			} else {
				atomic.AddInt64(&sucessos, 1)
			}
		}(i)
	}

	close(barreiraInicio) // Libera os passageiros
	wg.Wait()

	return MetricasSimulacao{
		TotalRequisicoes: int64(quantidadePassageiros),
		Sucessos:         sucessos,
		FalhasOuAbortos:  falhas,
		TempoTotal:       time.Since(inicio),
	}
}

type roundTripperRoteador struct {
	mu         sync.RWMutex
	servidores map[string]http.Handler
}

func (r *roundTripperRoteador) DesligarServidor(urlBase string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.servidores, urlBase)
}

func (r *roundTripperRoteador) RoundTrip(req *http.Request) (*http.Response, error) {
	urlBase := fmt.Sprintf("%s://%s", req.URL.Scheme, req.URL.Host)
	r.mu.RLock()
	handler, existe := r.servidores[urlBase]
	r.mu.RUnlock()

	if !existe {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       http.NoBody,
		}, nil
	}

	recorder := &respostaGravador{
		header: make(http.Header),
		body:   new(bytesBuffer),
		code:   http.StatusOK,
	}
	handler.ServeHTTP(recorder, req)

	return recorder.Result(), nil
}

type bytesBuffer struct {
	data []byte
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesBuffer) Read(p []byte) (n int, err error) {
	if len(b.data) == 0 {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, b.data)
	b.data = b.data[n:]
	return n, nil
}

func (b *bytesBuffer) Close() error { return nil }

type respostaGravador struct {
	header http.Header
	body   *bytesBuffer
	code   int
}

func (r *respostaGravador) Header() http.Header         { return r.header }
func (r *respostaGravador) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *respostaGravador) WriteHeader(statusCode int)  { r.code = statusCode }
func (r *respostaGravador) Result() *http.Response {
	return &http.Response{
		StatusCode: r.code,
		Header:     r.header,
		Body:       r.body,
	}
}
