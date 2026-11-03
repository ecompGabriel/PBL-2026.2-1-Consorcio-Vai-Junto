package config

import (
	"os"
	"testing"
)

// Testa o carregamento com configurações válidas e parceiros bem formatados.
func TestCarregarConfigValida(t *testing.T) {
	os.Setenv("VAIJUNTO_EMPRESA_ID", "empresa-a")
	os.Setenv("VAIJUNTO_TCP_LISTEN", ":3081")
	os.Setenv("VAIJUNTO_REST_LISTEN", ":3091")
	os.Setenv("VAIJUNTO_PARCEIROS", "empresa-b=http://empresa-b:3090,empresa-c=http://empresa-c:3090")

	defer func() {
		os.Unsetenv("VAIJUNTO_EMPRESA_ID")
		os.Unsetenv("VAIJUNTO_TCP_LISTEN")
		os.Unsetenv("VAIJUNTO_REST_LISTEN")
		os.Unsetenv("VAIJUNTO_PARCEIROS")
	}()

	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("não esperava erro ao carregar config válida: %v", err)
	}

	if cfg.EmpresaID != "empresa-a" {
		t.Errorf("EmpresaID = %s; esperava empresa-a", cfg.EmpresaID)
	}
	if cfg.TCPListen != ":3081" {
		t.Errorf("TCPListen = %s; esperava :3081", cfg.TCPListen)
	}
	if cfg.RESTListen != ":3091" {
		t.Errorf("RESTListen = %s; esperava :3091", cfg.RESTListen)
	}
	if len(cfg.Parceiros) != 2 {
		t.Fatalf("quantidade de parceiros = %d; esperava 2", len(cfg.Parceiros))
	}

	empB, existe := cfg.Parceiros["empresa-b"]
	if !existe || empB.URLBase != "http://empresa-b:3090" {
		t.Errorf("parceiro empresa-b incorreto: %+v", empB)
	}
}

// Testa a obrigatoriedade da identificação da empresa via VAIJUNTO_EMPRESA_ID.
func TestCarregarRejeitaEmpresaIDVazio(t *testing.T) {
	os.Unsetenv("VAIJUNTO_EMPRESA_ID")

	_, err := Carregar()
	if err == nil {
		t.Error("esperava erro ao carregar config sem VAIJUNTO_EMPRESA_ID, mas retornou nil")
	}
}

// Testa a rejeição quando um nó tenta listar a si mesmo como empresa parceira.
func TestCarregarRejeitaParceiroComPropriaEmpresa(t *testing.T) {
	os.Setenv("VAIJUNTO_EMPRESA_ID", "empresa-a")
	os.Setenv("VAIJUNTO_PARCEIROS", "empresa-a=http://localhost:3090")
	defer func() {
		os.Unsetenv("VAIJUNTO_EMPRESA_ID")
		os.Unsetenv("VAIJUNTO_PARCEIROS")
	}()

	_, err := Carregar()
	if err == nil {
		t.Error("esperava erro ao listar a própria empresa como parceira, mas retornou nil")
	}
}

// Testa a validação de URLs malformadas ou com esquemas inválidos.
func TestCarregarRejeitaURLInvalida(t *testing.T) {
	os.Setenv("VAIJUNTO_EMPRESA_ID", "empresa-a")
	os.Setenv("VAIJUNTO_PARCEIROS", "empresa-b=url-sem-esquema-http")
	defer func() {
		os.Unsetenv("VAIJUNTO_EMPRESA_ID")
		os.Unsetenv("VAIJUNTO_PARCEIROS")
	}()

	_, err := Carregar()
	if err == nil {
		t.Error("esperava erro para parceiro com URL inválida, mas retornou nil")
	}
}

// Testa a detecção e rejeição de entradas duplicadas na lista de parceiros.
func TestCarregarRejeitaParceiroDuplicado(t *testing.T) {
	os.Setenv("VAIJUNTO_EMPRESA_ID", "empresa-a")
	os.Setenv("VAIJUNTO_PARCEIROS", "empresa-b=http://empresa-b:3090,empresa-b=http://empresa-b-2:3090")
	defer func() {
		os.Unsetenv("VAIJUNTO_EMPRESA_ID")
		os.Unsetenv("VAIJUNTO_PARCEIROS")
	}()

	_, err := Carregar()
	if err == nil {
		t.Error("esperava erro para parceiro duplicado, mas retornou nil")
	}
}
