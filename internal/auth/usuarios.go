package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"sync"
)

type usuario struct {
	// A senha pura não é armazenada: somente salt e hash permanecem na memória.
	perfil    string
	salt      []byte
	senhaHash [sha256.Size]byte
}

type GerenciadorUsuarios struct {
	// Usuários também são estado compartilhado: cadastro e autenticação não
	// podem ler ou alterar o mapa simultaneamente sem sincronização.
	mu       sync.Mutex
	usuarios map[string]usuario
}

func NovoGerenciadorUsuarios() *GerenciadorUsuarios {
	return &GerenciadorUsuarios{usuarios: make(map[string]usuario)}
}

func (gerenciador *GerenciadorUsuarios) Registrar(id string, senha string, perfil string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("o ID do usuário é obrigatório")
	}
	if len(senha) < 4 {
		return errors.New("a senha deve possuir ao menos 4 caracteres")
	}
	if perfil != "motorista" && perfil != "passageiro" {
		return errors.New("perfil inválido")
	}

	gerenciador.mu.Lock()
	defer gerenciador.mu.Unlock()

	if _, existe := gerenciador.usuarios[id]; existe {
		return errors.New("já existe um usuário com esse ID")
	}

	// Um salt aleatório faz com que senhas iguais não tenham o mesmo hash.
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		return errors.New("não foi possível proteger a senha")
	}

	gerenciador.usuarios[id] = usuario{
		perfil:    perfil,
		salt:      salt,
		senhaHash: calcularHash(salt, senha),
	}

	return nil
}

func (gerenciador *GerenciadorUsuarios) Autenticar(id string, senha string) (string, error) {
	id = strings.TrimSpace(id)

	gerenciador.mu.Lock()
	defer gerenciador.mu.Unlock()

	usuarioEncontrado, existe := gerenciador.usuarios[id]
	if !existe {
		return "", errors.New("usuário ou senha inválidos")
	}

	hashInformado := calcularHash(usuarioEncontrado.salt, senha)
	// A comparação em tempo constante reduz a exposição a ataques por tempo.
	if subtle.ConstantTimeCompare(hashInformado[:], usuarioEncontrado.senhaHash[:]) != 1 {
		return "", errors.New("usuário ou senha inválidos")
	}

	return usuarioEncontrado.perfil, nil
}

func calcularHash(salt []byte, senha string) [sha256.Size]byte {
	// Função centralizada para garantir o mesmo cálculo no cadastro e no login.
	dados := make([]byte, 0, len(salt)+len(senha))
	dados = append(dados, salt...)
	dados = append(dados, []byte(senha)...)
	return sha256.Sum256(dados)
}
