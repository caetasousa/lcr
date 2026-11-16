package domain

import (
	"bytes"
	"time"
)

// Retangulo em pontos PDF (1/72 pol.), origem no canto inferior esquerdo.
type Retangulo struct{ X1, Y1, X2, Y2 float64 }

// Tamanho mínimo do carimbo: abaixo disso o texto fica ilegível.
const (
	larguraMinima = 40
	alturaMinima  = 20
)

// NovaPosicao normaliza os cantos e valida página e tamanho.
func NovaPosicao(pagina int, x1, y1, x2, y2 float64) (Posicao, error) {
	if pagina < 1 {
		return Posicao{}, ErrPosicaoInvalida
	}
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if x2-x1 < larguraMinima || y2-y1 < alturaMinima || x1 < 0 || y1 < 0 {
		return Posicao{}, ErrPosicaoInvalida
	}
	return Posicao{Pagina: pagina, Retangulo: Retangulo{x1, y1, x2, y2}}, nil
}

// Posicao do carimbo visual.
type Posicao struct {
	Pagina    int
	Retangulo Retangulo
}

// PDF é um value object para "bytes que dizem ser um PDF".
type PDF []byte

func NovoPDF(b []byte) (PDF, error) {
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return nil, ErrPDFInvalido
	}
	return PDF(b), nil
}

// Documento assinado e arquivado. No banco ele fica sempre cifrado
// (Conteudo); PDFAssinado só existe em memória, depois de decifrado e de o
// SHA-256 ter sido conferido.
type Documento struct {
	ID             int64
	UsuarioID      int64 // quem fez esta assinatura
	CertificadoID  int64
	NomeArquivo    string
	SHA256Original string
	SHA256Assinado string
	Pagina         int
	AssinadoEm     time.Time
	PDFAssinado    []byte   // em claro (só em memória)
	Conteudo       []byte   // cifrado com AES-256-GCM (o que vai para o banco)
	Tamanho        int      // tamanho do PDF em claro
	Signatarios    []string // nomes de todos que têm acesso (todos que assinaram)
}

// ChaveDocumento é a chave AES do documento cifrada (RSA-OAEP) para a chave
// pública de um certificado. Existe uma por signatário: é ela que dá acesso.
type ChaveDocumento struct {
	CertificadoID int64
	ChaveCifrada  []byte
}

// ConfereIntegridade garante a fidelidade: o PDF decifrado precisa ter
// exatamente o SHA-256 registrado no momento da assinatura.
func (d *Documento) ConfereIntegridade(pdf []byte, sha256Hex func([]byte) string) error {
	if sha256Hex(pdf) != d.SHA256Assinado {
		return ErrDocumentoCorrompido
	}
	return nil
}
