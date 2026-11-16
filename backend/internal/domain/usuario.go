package domain

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// CPF é um value object: só existe se os dígitos verificadores conferem.
// Guardado sempre com 11 dígitos, sem pontuação.
type CPF string

// NovoCPF aceita o CPF com ou sem máscara e valida os dois dígitos
// verificadores (módulo 11).
func NovoCPF(entrada string) (CPF, error) {
	d := make([]int, 0, 11)
	for _, r := range entrada {
		if r >= '0' && r <= '9' {
			d = append(d, int(r-'0'))
		}
	}
	if len(d) != 11 || todosIguais(d) {
		return "", ErrCPFInvalido
	}
	for n := 9; n <= 10; n++ {
		soma := 0
		for i := range n {
			soma += d[i] * (n + 1 - i)
		}
		if dv := (soma * 10) % 11 % 10; dv != d[n] {
			return "", ErrCPFInvalido
		}
	}
	var b strings.Builder
	for _, x := range d {
		b.WriteByte(byte('0' + x))
	}
	return CPF(b.String()), nil
}

func todosIguais(d []int) bool {
	for _, x := range d[1:] {
		if x != d[0] {
			return false
		}
	}
	return true
}

// Mascarado oculta parte do CPF, como nos carimbos de assinatura reais.
func (c CPF) Mascarado() string {
	s := string(c)
	if len(s) != 11 {
		return s
	}
	return "***." + s[3:6] + "." + s[6:9] + "-**"
}

// Formatado devolve 000.000.000-00.
func (c CPF) Formatado() string {
	s := string(c)
	if len(s) != 11 {
		return s
	}
	return s[0:3] + "." + s[3:6] + "." + s[6:9] + "-" + s[9:11]
}

// Usuario é o titular de certificados.
type Usuario struct {
	ID        int64
	Nome      string
	CPF       CPF
	SenhaHash string
	CriadoEm  time.Time
}

const (
	tamanhoMinimoSenha = 8
	// O teto evita que alguém mande senhas gigantes só para gastar CPU.
	tamanhoMaximoSenha = 128
)

// NormalizaNome remove espaços duplicados e valida tamanho e caracteres.
// Só letras (com acento), espaço, apóstrofo, hífen e ponto. Isso importa
// para a segurança: o nome vai para o CN do certificado no formato
// "NOME:CPF", então um ":" ou dígitos no nome permitiriam forjar um CN que
// aparenta ter o CPF de outra pessoa.
func NormalizaNome(nome string) (string, error) {
	n := strings.Join(strings.Fields(nome), " ")
	if c := utf8.RuneCountInString(n); c < 3 || c > 80 {
		return "", ErrNomeInvalido
	}
	for _, r := range n {
		if !unicode.IsLetter(r) && !unicode.IsMark(r) && !strings.ContainsRune(" '-.", r) {
			return "", ErrNomeInvalido
		}
	}
	return n, nil
}

// ValidaSenhaNova aplica a política de senha.
func ValidaSenhaNova(senha string) error {
	if c := utf8.RuneCountInString(senha); c < tamanhoMinimoSenha || c > tamanhoMaximoSenha {
		return ErrSenhaFraca
	}
	return nil
}

// PrimeiroNome é usado na saudação da interface.
func (u *Usuario) PrimeiroNome() string {
	if f := strings.Fields(u.Nome); len(f) > 0 {
		return f[0]
	}
	return u.Nome
}
