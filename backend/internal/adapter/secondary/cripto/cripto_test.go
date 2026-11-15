package cripto

import (
	"bytes"
	"errors"
	"testing"

	"lcr/internal/domain"
)

// Parâmetros leves para os testes rodarem rápido.
func novo(t *testing.T) *Argon2 {
	t.Helper()
	a, err := NovoArgon2(Parametros{Tempo: 1, Memoria: 8 * 1024, Threads: 1, TamChave: 32})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestHashSenha(t *testing.T) {
	a := novo(t)
	h, err := a.Gerar("correta-horse")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Conferir("correta-horse", h) {
		t.Error("senha correta foi recusada")
	}
	if a.Conferir("errada", h) {
		t.Error("senha errada foi aceita")
	}
	h2, _ := a.Gerar("correta-horse")
	if h == h2 {
		t.Error("dois hashes da mesma senha não podem ser iguais (salt)")
	}
}

func TestCofre(t *testing.T) {
	a := novo(t)
	segredo := []byte("chave privada PKCS#8")
	blob, err := a.Selar(segredo, "senha123", "ctx")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, segredo) {
		t.Fatal("o segredo aparece em claro no blob")
	}
	got, err := a.Abrir(blob, "senha123", "ctx")
	if err != nil || !bytes.Equal(got, segredo) {
		t.Fatalf("Abrir = %q, %v", got, err)
	}

	casos := map[string]func() ([]byte, error){
		"senha errada":    func() ([]byte, error) { return a.Abrir(blob, "outra", "ctx") },
		"contexto errado": func() ([]byte, error) { return a.Abrir(blob, "senha123", "outro") },
		"blob adulterado": func() ([]byte, error) {
			b := bytes.Clone(blob)
			b[len(b)-1] ^= 1
			return a.Abrir(b, "senha123", "ctx")
		},
	}
	for nome, f := range casos {
		if _, err := f(); !errors.Is(err, domain.ErrSenhaIncorreta) {
			t.Errorf("%s: esperado ErrSenhaIncorreta, veio %v", nome, err)
		}
	}
}

func TestParametrosFracos(t *testing.T) {
	if _, err := NovoArgon2(Parametros{Tempo: 1, Memoria: 1024, Threads: 1, TamChave: 32}); err == nil {
		t.Error("deveria recusar memória de 1 MiB")
	}
}

func TestTokens(t *testing.T) {
	tok, h, err := Tokens{}.Novo()
	if err != nil || len(tok) != 64 {
		t.Fatalf("token %q, %v", tok, err)
	}
	if !bytes.Equal(h, Tokens{}.Hash(tok)) {
		t.Error("hash do token não é determinístico")
	}
}
