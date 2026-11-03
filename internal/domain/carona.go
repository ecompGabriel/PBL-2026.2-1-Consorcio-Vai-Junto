package domain

import (
	"errors"
	"strings"
	"time"
)

type Carona struct {
	// Rota e trechos ficam privados para que somente métodos do domínio alterem
	// a disponibilidade de assentos de cada segmento.
	ID           string    // Identificador informado pelo motorista na publicação.
	empresaID    string    // Empresa dona da carona e de seus assentos no consórcio.
	motoristaID  string    // Dono da carona; vem da sessão, nunca do JSON livre.
	horarioSaida time.Time // Horário de início do primeiro trecho da rota.
	rota         []string  // Sequência ordenada de cidades informada pelo motorista.
	trechos      []Trecho  // Segmentos consecutivos que realmente controlam vagas.
	cancelada    bool      // Impede novas buscas e reservas sem apagar o histórico.
}

func (carona Carona) Trechos() []Trecho {
	// Retorna uma cópia para o chamador não alterar os trechos internos.
	return append([]Trecho(nil), carona.trechos...)
}

func (carona Carona) EmpresaID() string {
	// Expõe a empresa detentora da carona no consórcio.
	return carona.empresaID
}

func (carona Carona) MotoristaID() string {
	// Expõe o dono sem tornar o campo interno modificável por outros pacotes.
	return carona.motoristaID
}

// EstaAtiva informa se a carona ainda pode aparecer em buscas e receber reservas.
func (carona Carona) EstaAtiva() bool {
	return !carona.cancelada
}

func (carona Carona) EstaCancelada() bool {
	return carona.cancelada
}

func (carona *Carona) Cancelar() error {
	// O estado é preservado para que consultas e reservas antigas continuem
	// existindo, porém a carona deixa de aceitar novas confirmações.
	if carona.cancelada {
		return errors.New("a carona já está cancelada")
	}

	carona.cancelada = true
	return nil
}

func (carona Carona) TemVagasNoTrecho(ordem int, quantidade int) bool {
	// A ordem é a posição do trecho dentro da rota, por exemplo 0 para A→B e 1
	// para B→C. Ela evita depender dos nomes das cidades para identificar vagas.
	for _, trecho := range carona.trechos {
		if trecho.Ordem == ordem {
			return trecho.TemVagas(quantidade)
		}
	}

	return false
}

// PodeCancelarNoTrecho é usado pelo catálogo antes de devolver vários assentos
// de uma vez, mantendo a disponibilidade dentro da capacidade do trecho.
func (carona Carona) PodeCancelarNoTrecho(ordem int, quantidade int) bool {
	for _, trecho := range carona.trechos {
		if trecho.Ordem == ordem {
			return quantidade > 0 && trecho.AssentosDisponiveis+quantidade <= trecho.Capacidade
		}
	}

	return false
}

func (carona *Carona) ReservarNoTrecho(ordem int, quantidade int) error {
	// Este método altera somente o trecho escolhido; o mutex do catálogo deve
	// estar protegido antes de chamá-lo em uma operação do servidor.
	trecho, err := carona.encontrarTrechoPorOrdem(ordem)
	if err != nil {
		return err
	}

	return trecho.Reservar(quantidade)
}

func (carona *Carona) CancelarNoTrecho(ordem int, quantidade int) error {
	// A devolução também é localizada no trecho, pois assentos não pertencem à
	// carona inteira quando há cidades intermediárias.
	trecho, err := carona.encontrarTrechoPorOrdem(ordem)
	if err != nil {
		return err
	}

	return trecho.CancelarReserva(quantidade)
}

func (carona *Carona) encontrarTrechoPorOrdem(ordem int) (*Trecho, error) {
	for indice := range carona.trechos {
		if carona.trechos[indice].Ordem == ordem {
			return &carona.trechos[indice], nil
		}
	}

	return nil, errors.New("trecho não encontrado na carona")
}

func (carona Carona) TemCidade(cidade string) bool {
	// É uma consulta simples da rota, sem alterar seus dados.
	for _, c := range carona.rota {
		if c == cidade {
			return true
		}
	}
	return false
}

func (carona Carona) PosicaoCidade(cidade string) (int, bool) {
	// O booleano diferencia a cidade na posição zero de uma cidade inexistente.
	for i, c := range carona.rota {
		if c == cidade {
			return i, true
		}
	}
	return 0, false
}

func (carona Carona) TrechosEntre(origem string, destino string) ([]Trecho, error) {
	// A fatia começa na origem e termina antes do índice do destino: em A-B-C,
	// viajar de A até C ocupa os trechos A→B e B→C.
	posicaoOrigem, encontrouOrigem := carona.PosicaoCidade(origem)
	posicaoDestino, encontrouDestino := carona.PosicaoCidade(destino)

	if !encontrouOrigem || !encontrouDestino {
		return nil, errors.New("origem ou destino não pertencem à rota")
	}

	if posicaoOrigem >= posicaoDestino {
		return nil, errors.New("a origem deve estar antes do destino na rota")
	}

	return carona.trechos[posicaoOrigem:posicaoDestino], nil
}

func (carona Carona) PrecosEntre(origem string, destino string) (int64, error) {
	// O preço final é a soma dos preços dos trechos efetivamente ocupados.
	trechos, err := carona.TrechosEntre(origem, destino)
	if err != nil {
		return 0, err
	}

	var precoTotal int64
	for _, trecho := range trechos {
		precoTotal += trecho.PrecoCentavos
	}
	return precoTotal, nil
}

func NovaCarona(
	empresaID string,
	id string,
	motoristaID string,
	horarioSaida time.Time,
	rota []string,
	capacidade int,
	precosCentavos []int64,
	duracoesMinutos []int,
) (*Carona, error) {
	// A construtora concentra as validações e cria uma carona sempre coerente;
	// nenhum outro pacote monta trechos manualmente para uma publicação normal.
	if strings.TrimSpace(empresaID) == "" {
		return nil, errors.New("o ID da empresa é obrigatório")
	}

	if len(rota) < 2 {
		return nil, errors.New("a rota precisa ter ao menos duas cidades")
	}

	if capacidade <= 0 {
		return nil, errors.New("a capacidade precisa ser maior que zero")
	}

	quantidadeTrechos := len(rota) - 1
	if len(precosCentavos) != quantidadeTrechos {
		return nil, errors.New("a quantidade de preços deve ser igual à quantidade de trechos")
	}
	if len(duracoesMinutos) != quantidadeTrechos {
		return nil, errors.New("a quantidade de durações deve ser igual à quantidade de trechos")
	}

	trechos := make([]Trecho, 0, quantidadeTrechos)
	horarioTrecho := horarioSaida
	// Cidades adjacentes da rota geram trechos independentes: A→B e B→C têm
	// disponibilidade própria, mesmo pertencendo à mesma carona.
	for i := 0; i < quantidadeTrechos; i++ {
		if precosCentavos[i] < 0 {
			return nil, errors.New("o preço de um trecho não pode ser negativo")
		}
		if duracoesMinutos[i] <= 0 {
			return nil, errors.New("a duração de um trecho deve ser maior que zero")
		}

		// A chegada deste trecho se torna a saída do próximo trecho da mesma rota.
		horarioChegada := horarioTrecho.Add(time.Duration(duracoesMinutos[i]) * time.Minute)

		trechos = append(trechos, Trecho{
			EmpresaID:           empresaID,
			CaronaID:            id,
			Ordem:               i,
			Origem:              rota[i],
			Destino:             rota[i+1],
			Capacidade:          capacidade,
			AssentosDisponiveis: capacidade,
			PrecoCentavos:       precosCentavos[i],
			HorarioSaida:        horarioTrecho,
			HorarioChegada:      horarioChegada,
		})
		horarioTrecho = horarioChegada
	}

	return &Carona{
		ID:           id,
		empresaID:    empresaID,
		motoristaID:  motoristaID,
		horarioSaida: horarioSaida,
		// Copia a rota recebida para manter o encapsulamento do domínio.
		rota:    append([]string(nil), rota...),
		trechos: trechos,
	}, nil
}
