package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type usuario struct {
	perfil    string
	salt      []byte
	senhaHash [sha256.Size]byte
}

type usuarioSnapshot struct {
	ID        string `json:"id"`
	Perfil    string `json:"perfil"`
	Salt      []byte `json:"salt"`
	SenhaHash []byte `json:"senha_hash"`
}

type GerenciadorUsuarios struct {
	mu              sync.Mutex
	usuarios        map[string]usuario
	arquivoSnapshot string
}

func NovoGerenciadorUsuarios() *GerenciadorUsuarios {
	return NovoGerenciadorUsuariosComPersistencia("")
}

func NovoGerenciadorUsuariosComPersistencia(caminhoArquivo string) *GerenciadorUsuarios {
	g := &GerenciadorUsuarios{
		usuarios:        make(map[string]usuario),
		arquivoSnapshot: caminhoArquivo,
	}
	if caminhoArquivo != "" {
		_ = g.carregarDoDisco()
	}
	return g
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
	_ = gerenciador.salvarNoDisco()

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

func (gerenciador *GerenciadorUsuarios) salvarNoDisco() error {
	if gerenciador.arquivoSnapshot == "" {
		return nil
	}

	slice := make([]usuarioSnapshot, 0, len(gerenciador.usuarios))
	for id, u := range gerenciador.usuarios {
		slice = append(slice, usuarioSnapshot{
			ID:        id,
			Perfil:    u.perfil,
			Salt:      u.salt,
			SenhaHash: u.senhaHash[:],
		})
	}

	dados, err := json.MarshalIndent(slice, "", "  ")
	if err != nil {
		return err
	}

	_ = os.MkdirAll(filepath.Dir(gerenciador.arquivoSnapshot), 0755)
	return os.WriteFile(gerenciador.arquivoSnapshot, dados, 0644)
}

func (gerenciador *GerenciadorUsuarios) carregarDoDisco() error {
	if gerenciador.arquivoSnapshot == "" {
		return nil
	}

	dados, err := os.ReadFile(gerenciador.arquivoSnapshot)
	if err != nil {
		return err
	}

	var slice []usuarioSnapshot
	err = json.Unmarshal(dados, &slice)
	if err != nil {
		return err
	}

	for _, item := range slice {
		var hash [sha256.Size]byte
		copy(hash[:], item.SenhaHash)
		gerenciador.usuarios[item.ID] = usuario{
			perfil:    item.Perfil,
			salt:      item.Salt,
			senhaHash: hash,
		}
	}
	return nil
}
