package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNovoCPF(t *testing.T) {
	casos := []struct {
		entrada string
		quer    CPF
		erro    bool
	}{
		{"529.982.247-25", "52998224725", false},
		{"52998224725", "52998224725", false},
		{"111.444.777-35", "11144477735", false},
		{"529.982.247-26", "", true}, // dígito verificador errado
		{"111.111.111-11", "", true}, // todos iguais
		{"1234", "", true},
		{"", "", true},
	}
	for _, c := range casos {
		got, err := NovoCPF(c.entrada)
		if (err != nil) != c.erro || got != c.quer {
			t.Errorf("NovoCPF(%q) = %q, %v", c.entrada, got, err)
		}
	}
	if m := CPF("52998224725").Mascarado(); m != "***.982.247-**" {
		t.Errorf("Mascarado = %q", m)
	}
}

func TestCicloDeVidaDoCertificado(t *testing.T) {
	agora := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	c := &Certificado{EmitidoEm: agora.AddDate(0, -1, 0), ExpiraEm: agora.AddDate(1, 0, 0)}

	if c.Situacao(agora) != SituacaoValido || c.PodeAssinar(agora) != nil {
		t.Fatal("certificado novo deveria ser válido e poder assinar")
	}
	if !errors.Is(c.PodeSerSubstituido(agora), ErrCertificadoAindaValido) {
		t.Error("não pode substituir um certificado válido")
	}
	if !errors.Is(c.Revogar(agora, MotivoRevogacao(2)), ErrMotivoInvalido) {
		t.Error("código 2 (CA compromise) não é aceito para usuários")
	}
	if err := c.Revogar(agora, MotivoChaveComprometida); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(c.PodeAssinar(agora), ErrCertificadoRevogado) {
		t.Error("revogado não pode assinar")
	}
	if !errors.Is(c.Revogar(agora, MotivoChaveComprometida), ErrCertificadoJaRevogado) {
		t.Error("revogar duas vezes deveria falhar")
	}
	if c.PodeSerSubstituido(agora) != nil {
		t.Error("revogado pode ser substituído")
	}

	expirado := &Certificado{ExpiraEm: agora.Add(-time.Hour)}
	if !errors.Is(expirado.PodeAssinar(agora), ErrCertificadoExpirado) {
		t.Error("expirado não pode assinar")
	}
}

func TestAvaliarRevogacao(t *testing.T) {
	assinou := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	base := ResultadoAssinatura{Integra: true, CadeiaConfiavel: true, DataAssinatura: &assinou}

	casos := []struct {
		nome   string
		lcr    ConsultaLCR
		valida bool
	}{
		{"não revogado", ConsultaLCR{}, true},
		{"revogado depois de assinar", ConsultaLCR{Revogado: true, RevogadoEm: assinou.Add(time.Hour)}, true},
		{"revogado antes de assinar", ConsultaLCR{Revogado: true, RevogadoEm: assinou.Add(-time.Hour)}, false},
	}
	for _, c := range casos {
		r := base
		r.Avaliar(c.lcr)
		if r.Valida != c.valida {
			t.Errorf("%s: Valida = %v", c.nome, r.Valida)
		}
	}

	alterado := base
	alterado.Integra = false
	alterado.Avaliar(ConsultaLCR{})
	if alterado.Valida {
		t.Error("documento alterado não pode ser válido")
	}
}

func TestNovaPosicao(t *testing.T) {
	p, err := NovaPosicao(1, 300, 110, 40, 40) // cantos invertidos
	if err != nil {
		t.Fatal(err)
	}
	if p.Retangulo != (Retangulo{40, 40, 300, 110}) {
		t.Errorf("retângulo não normalizado: %+v", p.Retangulo)
	}
	if _, err := NovaPosicao(0, 0, 0, 100, 100); err == nil {
		t.Error("página 0 deveria ser recusada")
	}
	if _, err := NovaPosicao(1, 0, 0, 10, 10); err == nil {
		t.Error("carimbo minúsculo deveria ser recusado")
	}
}

func TestNomeNaoPodeForjarOCNDoCertificado(t *testing.T) {
	// O CN é "NOME:CPF": um nome com ":" ou dígitos poderia imitar o CPF de outra pessoa.
	for _, ruim := range []string{"Maria Silva:11144477735", "Joao 11144477735", "Ana <script>", "Zé\x00Silva", "A/B", "ab"} {
		if _, err := NormalizaNome(ruim); err == nil {
			t.Errorf("nome %q deveria ser recusado", ruim)
		}
	}
	for _, bom := range []string{"Maria da Conceição", "José D'Ávila", "Ana-Clara Souza Jr.", "  João   Pereira "} {
		if _, err := NormalizaNome(bom); err != nil {
			t.Errorf("nome %q deveria ser aceito: %v", bom, err)
		}
	}
}

func TestSenhaTemMinimoEMaximo(t *testing.T) {
	if ValidaSenhaNova("1234567") == nil || ValidaSenhaNova(strings.Repeat("a", 129)) == nil {
		t.Error("senha curta demais ou gigante deveria ser recusada")
	}
	if ValidaSenhaNova("12345678") != nil || ValidaSenhaNova(strings.Repeat("a", 128)) != nil {
		t.Error("limites 8 e 128 deveriam ser aceitos")
	}
}

func TestNomeDeArquivoEMotivoSaoSaneados(t *testing.T) {
	casos := map[string]string{
		"contrato.pdf":                    "contrato.pdf",
		"../../etc/passwd":                "passwd.pdf",
		`C:\Users\x\nota fiscal.PDF`:      "nota fiscal.pdf",
		"a\"b\r\nSet-Cookie: x.pdf":       "a_bSet-Cookie_ x.pdf",
		"fatura\u202Efdp.exe":             "faturafdp.exe.pdf",
		"":                                "documento.pdf",
		"...":                             "documento.pdf",
		strings.Repeat("a", 300) + ".pdf": strings.Repeat("a", 100) + ".pdf",
	}
	for entrada, quer := range casos {
		if got := NomeArquivoSeguro(entrada); got != quer {
			t.Errorf("NomeArquivoSeguro(%q) = %q, queria %q", entrada, got, quer)
		}
	}
	if m := NormalizaMotivo("  Aceite\tdo\n contrato\x00 "); m != "Aceite do contrato" {
		t.Errorf("motivo: %q", m)
	}
	if m := NormalizaMotivo(strings.Repeat("x", 500)); len([]rune(m)) != 120 {
		t.Errorf("motivo deveria ser cortado em 120: %d", len([]rune(m)))
	}
}
