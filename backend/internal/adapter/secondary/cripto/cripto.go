// Package cripto implementa as portas HasherSenha, Cofre e GeradorToken
// com primitivas modernas: Argon2id (RFC 9106), AES-256-GCM e crypto/rand.
package cripto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"lcr/internal/domain"
	"lcr/internal/port"
)

// Parametros do Argon2id. O padrão segue a 2ª recomendação da RFC 9106
// (t=3, m=64 MiB), adequada a servidores com pouca memória.
type Parametros struct {
	Tempo    uint32
	Memoria  uint32 // KiB
	Threads  uint8
	TamChave uint32
}

var ParametrosPadrao = Parametros{Tempo: 3, Memoria: 64 * 1024, Threads: 4, TamChave: 32}

const (
	tamSalt     = 16
	versaoCofre = "v1"
)

// Argon2 implementa HasherSenha e Cofre.
type Argon2 struct{ p Parametros }

var (
	_ port.HasherSenha = (*Argon2)(nil)
	_ port.Cofre       = (*Argon2)(nil)
)

// NovoArgon2 valida os parâmetros na construção (fail fast).
func NovoArgon2(p Parametros) (*Argon2, error) {
	if p.Tempo < 1 || p.Memoria < 8*1024 || p.Threads < 1 || p.TamChave < 16 {
		return nil, errors.New("parâmetros Argon2id fracos demais")
	}
	return &Argon2{p: p}, nil
}

func (a *Argon2) deriva(senha string, salt []byte) []byte {
	return argon2.IDKey([]byte(senha), salt, a.p.Tempo, a.p.Memoria, a.p.Threads, a.p.TamChave)
}

func aleatorio(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

// Gerar produz um hash no formato PHC:
// $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
func (a *Argon2) Gerar(senha string) (string, error) {
	salt, err := aleatorio(tamSalt)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, a.p.Memoria, a.p.Tempo, a.p.Threads,
		b64.EncodeToString(salt), b64.EncodeToString(a.deriva(senha, salt))), nil
}

// Conferir recalcula com os parâmetros gravados no próprio hash e compara
// em tempo constante.
func (a *Argon2) Conferir(senha, phc string) bool {
	partes := strings.Split(phc, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err1 := b64.DecodeString(partes[4])
	esperado, err2 := b64.DecodeString(partes[5])
	if err1 != nil || err2 != nil {
		return false
	}
	calc := argon2.IDKey([]byte(senha), salt, t, m, p, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(calc, esperado) == 1
}

// Selar: chave = Argon2id(senha, salt); blob = "v1" | salt | nonce | AES-256-GCM(segredo).
// O contexto entra como dado autenticado (AAD).
func (a *Argon2) Selar(segredo []byte, senha, contexto string) ([]byte, error) {
	salt, err := aleatorio(tamSalt)
	if err != nil {
		return nil, err
	}
	gcm, err := a.gcm(senha, salt)
	if err != nil {
		return nil, err
	}
	nonce, err := aleatorio(gcm.NonceSize())
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(versaoCofre)+tamSalt+len(nonce)+len(segredo)+gcm.Overhead())
	out = append(append(append(out, versaoCofre...), salt...), nonce...)
	return gcm.Seal(out, nonce, segredo, []byte(contexto)), nil
}

func (a *Argon2) Abrir(blob []byte, senha, contexto string) ([]byte, error) {
	cab := len(versaoCofre) + tamSalt
	if len(blob) < cab+12+16 || string(blob[:len(versaoCofre)]) != versaoCofre {
		return nil, errors.New("blob selado em formato inválido")
	}
	gcm, err := a.gcm(senha, blob[len(versaoCofre):cab])
	if err != nil {
		return nil, err
	}
	resto := blob[cab:]
	nonce, ct := resto[:gcm.NonceSize()], resto[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, []byte(contexto))
	if err != nil {
		// Tag do GCM não conferiu: senha errada (ou blob adulterado).
		return nil, domain.ErrSenhaIncorreta
	}
	return pt, nil
}

func (a *Argon2) gcm(senha string, salt []byte) (cipher.AEAD, error) {
	bloco, err := aes.NewCipher(a.deriva(senha, salt))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(bloco)
}

// Tokens implementa GeradorToken: 256 bits aleatórios; o banco guarda só
// o SHA-256, então um vazamento da tabela não entrega sessões válidas.
type Tokens struct{}

var _ port.GeradorToken = Tokens{}

func (Tokens) Novo() (string, []byte, error) {
	b, err := aleatorio(32)
	if err != nil {
		return "", nil, err
	}
	t := hex.EncodeToString(b)
	return t, Tokens{}.Hash(t), nil
}

func (Tokens) Hash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
