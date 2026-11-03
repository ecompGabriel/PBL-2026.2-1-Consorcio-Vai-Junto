package domain

type Itinerario struct {
	// Um itinerário pode conter trechos de uma ou de várias caronas.
	Origem             string
	Destino            string
	Trechos            []Trecho
	PrecoTotalCentavos int64
}

func (i Itinerario) CalcularPrecoTotal() int64 {
	// Os valores usam centavos para evitar imprecisões de ponto flutuante.
	var precoTotal int64

	for _, trecho := range i.Trechos {
		precoTotal += trecho.PrecoCentavos
	}
	return precoTotal
}

func (i Itinerario) TemVagas(quantidade int) bool {
	// Um itinerário só é viável se todos os seus trechos comportarem a mesma
	// quantidade solicitada pelo passageiro.
	for _, trecho := range i.Trechos {
		if !trecho.TemVagas(quantidade) {
			return false
		}
	}
	return true
}
