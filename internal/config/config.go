package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	PortaTCPPadrao  = ":3080"
	PortaRESTPadrao = ":3090"
)

// Parceiro representa uma empresa vizinha no consórcio.
type Parceiro struct {
	ID      string
	URLBase string // Exemplo: "http://empresa-b:3090"
}

// Config armazena a identidade do nó atual e os dados de conectividade.
type Config struct {
	EmpresaID  string
	TCPListen  string
	RESTListen string
	Parceiros  map[string]Parceiro // Chave é o ID da empresa parceira
}

// Carregar lê as variáveis de ambiente e valida as configurações do nó.
func Carregar() (*Config, error) {
	empresaID := strings.TrimSpace(os.Getenv("VAIJUNTO_EMPRESA_ID"))
	if empresaID == "" {
		return nil, errors.New("variável de ambiente VAIJUNTO_EMPRESA_ID é obrigatória")
	}

	tcpListen := strings.TrimSpace(os.Getenv("VAIJUNTO_TCP_LISTEN"))
	if tcpListen == "" {
		tcpListen = PortaTCPPadrao
	}

	restListen := strings.TrimSpace(os.Getenv("VAIJUNTO_REST_LISTEN"))
	if restListen == "" {
		restListen = PortaRESTPadrao
	}

	parceirosTexto := strings.TrimSpace(os.Getenv("VAIJUNTO_PARCEIROS"))
	parceiros, err := parseParceiros(parceirosTexto, empresaID)
	if err != nil {
		return nil, err
	}

	return &Config{
		EmpresaID:  empresaID,
		TCPListen:  tcpListen,
		RESTListen: restListen,
		Parceiros:  parceiros,
	}, nil
}

// parseParceiros converte a string "id1=url1,id2=url2" em um mapa validado.
func parseParceiros(texto string, minhaEmpresaID string) (map[string]Parceiro, error) {
	parceiros := make(map[string]Parceiro)
	if texto == "" {
		return parceiros, nil
	}

	entradas := strings.Split(texto, ",")
	for _, entrada := range entradas {
		entrada = strings.TrimSpace(entrada)
		if entrada == "" {
			continue
		}

		partes := strings.SplitN(entrada, "=", 2)
		if len(partes) != 2 {
			return nil, fmt.Errorf("formato inválido para parceiro: %q (esperado ID=URL)", entrada)
		}

		id := strings.TrimSpace(partes[0])
		rawURL := strings.TrimRight(strings.TrimSpace(partes[1]), "/")

		if id == "" {
			return nil, fmt.Errorf("ID do parceiro não pode ser vazio em %q", entrada)
		}
		if id == minhaEmpresaID {
			return nil, fmt.Errorf("a própria empresa %q não pode ser listada como sua parceira", id)
		}

		parsedURL, err := url.ParseRequestURI(rawURL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return nil, fmt.Errorf("URL inválida para o parceiro %q: %q", id, rawURL)
		}

		_, jaExiste := parceiros[id]

		if jaExiste {
			return nil, fmt.Errorf("parceiro duplicado: %q", id)
		}

		parceiros[id] = Parceiro{
			ID:      id,
			URLBase: rawURL,
		}
	}

	return parceiros, nil
}