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

// Documento assinado e arquivado.
type Documento struct {
	ID             int64
	UsuarioID      int64
	CertificadoID  int64
	NomeArquivo    string
	SHA256Original string
	SHA256Assinado string
	Pagina         int
	AssinadoEm     time.Time
	PDFAssinado    []byte
	Tamanho        int
}
