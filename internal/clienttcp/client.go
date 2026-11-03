package clienttcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"vaijunto/internal/protocol"
)

const enderecoPadrao = "127.0.0.1:3080"

type Cliente struct {
	// Cliente concentra a conexão e os codificadores compartilhados pelos dois
	// programas de terminal. Cada instância representa uma sessão TCP.
	conexao  net.Conn
	encoder  *json.Encoder
	decoder  *json.Decoder
	contador uint64
}

func Conectar() (*Cliente, error) {
	// A variável permite que o mesmo cliente aponte para outro computador sem
	// alterar o código; sem ela, o teste continua local.
	endereco := os.Getenv("VAIJUNTO_SERVER")
	if endereco == "" {
		endereco = enderecoPadrao
	}

	// O timeout limita somente a tentativa inicial de conexão; não limita a
	// duração total da sessão depois que ela for estabelecida.
	conexao, err := net.DialTimeout("tcp", endereco, 10*time.Second)
	if err != nil {
		return nil, err
	}

	return &Cliente{
		conexao: conexao,
		encoder: json.NewEncoder(conexao),
		decoder: json.NewDecoder(conexao),
	}, nil
}

func (cliente *Cliente) Fechar() error {
	// O fechamento informa ao servidor que a sessão desta conexão terminou.
	return cliente.conexao.Close()
}

func (cliente *Cliente) RegistrarUsuario(usuarioID string, senha string, perfil string) (protocol.Resposta, error) {
	// Atalho para não repetir a montagem do corpo de cadastro nos dois clientes.
	dados := protocol.RegistrarUsuario{
		UsuarioID: usuarioID,
		Senha:     senha,
		Perfil:    perfil,
	}

	return cliente.Enviar("registrar_usuario", dados, nil)
}

func (cliente *Cliente) IniciarSessao(usuarioID string, senha string) (protocol.Resposta, error) {
	// A sessão será associada pelo servidor a esta conexão TCP específica.
	dados := protocol.IniciarSessao{
		UsuarioID: usuarioID,
		Senha:     senha,
	}

	return cliente.Enviar("iniciar_sessao", dados, nil)
}

func (cliente *Cliente) Enviar(operacao string, dados any, destino any) (protocol.Resposta, error) {
	// "dados" é o corpo específico da requisição; "destino", quando existe,
	// recebe o corpo específico de uma resposta de sucesso.
	dadosJSON, err := json.Marshal(dados)
	if err != nil {
		return protocol.Resposta{}, err
	}

	// O ID de correlação permite confirmar que a resposta recebida pertence a
	// esta requisição. O contador atômico evita repetição em uso concorrente.
	id := fmt.Sprintf("req-%d", atomic.AddUint64(&cliente.contador, 1))
	requisicao := protocol.Requisicao{
		Versao:   protocol.VersaoAtual,
		ID:       id,
		Operacao: operacao,
		Dados:    dadosJSON,
	}

	// Timeouts evitam que a interface fique bloqueada para sempre em uma falha
	// de rede ou em um servidor que deixou de responder.
	err = cliente.conexao.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err != nil {
		return protocol.Resposta{}, err
	}

	err = cliente.encoder.Encode(requisicao)
	if err != nil {
		return protocol.Resposta{}, err
	}

	err = cliente.conexao.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err != nil {
		return protocol.Resposta{}, err
	}

	var resposta protocol.Resposta
	err = cliente.decoder.Decode(&resposta)
	if err != nil {
		return protocol.Resposta{}, err
	}

	if resposta.Versao != protocol.VersaoAtual {
		return protocol.Resposta{}, errors.New("servidor respondeu com versão incompatível")
	}

	if resposta.ID != id {
		return protocol.Resposta{}, errors.New("ID de correlação da resposta não corresponde à requisição")
	}

	if resposta.Sucesso && destino != nil && len(resposta.Dados) > 0 {
		err = json.Unmarshal(resposta.Dados, destino)
		if err != nil {
			return protocol.Resposta{}, err
		}
	}

	return resposta, nil
}
