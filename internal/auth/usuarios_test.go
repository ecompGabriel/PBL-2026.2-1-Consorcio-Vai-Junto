package auth

import "testing"

func TestGerenciadorRegistraEAutenticaUsuario(t *testing.T) {
	gerenciador := NovoGerenciadorUsuarios()

	err := gerenciador.Registrar("gabriel", "senha123", "passageiro")
	if err != nil {
		t.Fatalf("não esperava erro ao registrar: %v", err)
	}

	perfil, err := gerenciador.Autenticar("gabriel", "senha123")
	if err != nil {
		t.Fatalf("não esperava erro ao autenticar: %v", err)
	}
	if perfil != "passageiro" {
		t.Errorf("perfil = %s; esperava passageiro", perfil)
	}
}

func TestGerenciadorRejeitaSenhaIncorreta(t *testing.T) {
	gerenciador := NovoGerenciadorUsuarios()
	err := gerenciador.Registrar("gabriel", "senha123", "motorista")
	if err != nil {
		t.Fatalf("não esperava erro ao registrar: %v", err)
	}

	_, err = gerenciador.Autenticar("gabriel", "outra-senha")
	if err == nil {
		t.Error("esperava erro para senha incorreta")
	}
}
