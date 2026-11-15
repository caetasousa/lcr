package pades

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/digitorus/pdfsign/sign"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"

	"lcr/internal/port"
)

// Paleta do carimbo, em RGB 0–1 (PDF). Segue a identidade visual: grafite
// #0C0C0C com acento lime #C4F042, legível em papel branco.
const (
	corFundo   = "0.975 0.98 0.955"
	corGrafite = "0.047 0.047 0.047"
	corLime    = "0.769 0.941 0.259"
	corSutil   = "0.38 0.38 0.38"
)

// pdfTexto converte para WinAnsi e escapa ( ) \ para uma string literal PDF.
// O encoder é criado por chamada: os encoders de x/text guardam estado e
// não podem ser compartilhados entre goroutines.
func pdfTexto(s string) string {
	b, err := encoding.ReplaceUnsupported(charmap.Windows1252.NewEncoder()).Bytes([]byte(s))
	if err != nil {
		b = []byte(s)
	}
	var out strings.Builder
	out.WriteByte('(')
	for _, c := range b {
		switch c {
		case '(', ')', '\\':
			out.WriteByte('\\')
			out.WriteByte(c)
		case '\r', '\n':
			out.WriteByte(' ')
		default:
			out.WriteByte(c)
		}
	}
	out.WriteByte(')')
	return out.String()
}

type linha struct {
	texto  string
	fonte  string
	escala float64
	cor    string
}

// carimbo desenha o quadro visível como um Form XObject em PDF puro
// (Helvetica, WinAnsi), sem imagens nem fontes embutidas.
func carimbo(p port.PedidoAssinaturaPDF) func(*sign.SignContext, [4]float64) ([]byte, error) {
	return func(_ *sign.SignContext, r [4]float64) ([]byte, error) {
		w, h := r[2]-r[0], r[3]-r[1]
		if w < 40 || h < 20 {
			return nil, errors.New("área da assinatura muito pequena")
		}
		linhas := []linha{
			{"DOCUMENTO ASSINADO DIGITALMENTE", "/F2", 0.78, corSutil},
			{strings.ToUpper(p.Nome), "/F2", 1.15, corGrafite},
			{"Data: " + p.Data.Format("02/01/2006 15:04:05-0700"), "/F1", 0.9, corGrafite},
			{"Verifique em " + p.URLValidar, "/F1", 0.8, corSutil},
		}

		barra := h * 0.075
		pad := h * 0.1
		x := barra + pad*1.2
		dispW := w - x - pad
		dispH := h - 2*pad

		// Fonte que cabe na altura (entrelinha 1.25) e na largura (largura
		// média aproximada dos glifos Helvetica).
		soma := 0.0
		for _, l := range linhas {
			soma += l.escala * 1.25
		}
		fs := dispH / soma
		for _, l := range linhas {
			fator := 0.53
			if l.fonte == "/F2" {
				fator = 0.68
			}
			n := float64(len([]rune(l.texto)))
			if n*fs*l.escala*fator > dispW {
				fs = dispW / (n * l.escala * fator)
			}
		}

		var s bytes.Buffer
		fmt.Fprintf(&s, "q\n%s rg\n0 0 %.2f %.2f re f\n", corFundo, w, h)
		fmt.Fprintf(&s, "%s RG 0.6 w\n0.3 0.3 %.2f %.2f re S\n", corGrafite, w-0.6, h-0.6)
		fmt.Fprintf(&s, "%s rg\n0 0 %.2f %.2f re f\n", corGrafite, barra, h)
		fmt.Fprintf(&s, "%s rg\n%.2f 0 %.2f %.2f re f\nQ\n", corLime, barra, barra*0.35, h)

		total := 0.0
		for _, l := range linhas {
			total += fs * l.escala * 1.25
		}
		y := h - (h-total)/2
		for _, l := range linhas {
			tam := fs * l.escala
			y -= tam * 1.25
			fmt.Fprintf(&s, "BT\n%s rg\n%s %.2f Tf\n%.2f %.2f Td\n%s Tj\nET\n",
				l.cor, l.fonte, tam, x, y+tam*0.25, pdfTexto(l.texto))
		}

		var obj bytes.Buffer
		obj.WriteString("<<\n/Type /XObject\n/Subtype /Form\n")
		fmt.Fprintf(&obj, "/BBox [0 0 %.2f %.2f]\n/Matrix [1 0 0 1 0 0]\n", w, h)
		obj.WriteString("/Resources << /Font <<\n")
		obj.WriteString("  /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\n")
		obj.WriteString("  /F2 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>\n")
		obj.WriteString(">> >>\n")
		fmt.Fprintf(&obj, "/FormType 1\n/Length %d\n>>\nstream\n", s.Len())
		obj.Write(s.Bytes())
		obj.WriteString("\nendstream\n")
		return obj.Bytes(), nil
	}
}
