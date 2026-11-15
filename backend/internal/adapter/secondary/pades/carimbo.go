package pades

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/digitorus/pdfsign/sign"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"

	"lcr/internal/port"
)

// Paleta do carimbo, em RGB 0–1 (PDF). Segue a identidade visual: grafite
// #0C0C0C com acento lime #C4F042, legível em papel branco.
const (
	corFundo   = "1 1 1"
	corBorda   = "0.80 0.80 0.80"
	corGrafite = "0.047 0.047 0.047"
	corLime    = "0.769 0.941 0.259"
	corSutil   = "0.42 0.42 0.42"
	corBranco  = "1 1 1"
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

// Larguras dos glifos (unidades de 1/1000 do corpo) das fontes padrão
// Helvetica e Helvetica-Bold, ASCII 32–126, tiradas das métricas AFM da Adobe.
// Com elas o texto é dimensionado para ocupar o espaço disponível sem vazar.
var (
	larguraHelvetica = [95]float64{
		278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, // espaço a /
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556, // 0-9
		278, 278, 584, 584, 584, 556, 1015, // : a @
		667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, // A-M
		722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, // N-Z
		278, 278, 278, 469, 556, 333, // [ a `
		556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, // a-m
		556, 556, 556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, // n-z
		334, 260, 334, 584, // { a ~
	}
	larguraHelveticaBold = [95]float64{
		278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556,
		333, 333, 584, 584, 584, 611, 975,
		722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833,
		722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611,
		333, 278, 333, 584, 556, 333,
		556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889,
		611, 611, 611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500,
		389, 280, 389, 584,
	}
	// Letras acentuadas têm a mesma largura da letra base.
	semAcento = strings.NewReplacer(
		"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A", "É", "E", "Ê", "E", "È", "E",
		"Í", "I", "Ì", "I", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O", "Ú", "U", "Ü", "U", "Ç", "C", "Ñ", "N",
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "ê", "e", "è", "e",
		"í", "i", "ì", "i", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ú", "u", "ü", "u", "ç", "c", "ñ", "n",
	)
)

type fonte struct {
	nome     string // recurso no XObject
	larguras *[95]float64
}

var (
	regular = fonte{"/F1", &larguraHelvetica}
	negrito = fonte{"/F2", &larguraHelveticaBold}
)

// largura do texto em pontos, no corpo tam, com espaçamento extra entre letras.
func (f fonte) largura(texto string, tam, espacamento float64) float64 {
	total, n := 0.0, 0
	for _, r := range semAcento.Replace(texto) {
		w := 556.0
		if r >= 32 && r <= 126 {
			w = f.larguras[r-32]
		}
		total += w
		n++
	}
	return total*tam/1000 + float64(max(n-1, 0))*espacamento
}

type linha struct {
	texto       string
	fonte       fonte
	escala      float64 // tamanho relativo ao corpo base
	cor         string
	espacamento float64 // espaçamento entre letras, em frações do corpo
}

// caminhoArredondado desenha um retângulo com cantos arredondados (curvas de
// Bézier). Com soTopo, só os cantos de cima são arredondados.
func caminhoArredondado(s *bytes.Buffer, x, y, w, h, r float64, soTopo bool) {
	const k = 0.5523 // aproximação de um quarto de círculo por Bézier
	rb := r
	if soTopo {
		rb = 0
	}
	fmt.Fprintf(s, "%.2f %.2f m\n", x+rb, y)
	fmt.Fprintf(s, "%.2f %.2f l\n", x+w-rb, y)
	if rb > 0 {
		fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", x+w-rb+rb*k, y, x+w, y+rb-rb*k, x+w, y+rb)
	}
	fmt.Fprintf(s, "%.2f %.2f l\n", x+w, y+h-r)
	fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", x+w, y+h-r+r*k, x+w-r+r*k, y+h, x+w-r, y+h)
	fmt.Fprintf(s, "%.2f %.2f l\n", x+r, y+h)
	fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", x+r-r*k, y+h, x, y+h-r+r*k, x, y+h-r)
	fmt.Fprintf(s, "%.2f %.2f l\n", x, y+rb)
	if rb > 0 {
		fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", x, y+rb-rb*k, x+rb-rb*k, y, x+rb, y)
	}
	s.WriteString("h\n")
}

// circulo desenha um círculo de centro (cx, cy) e raio r.
func circulo(s *bytes.Buffer, cx, cy, r float64) {
	const k = 0.5523
	fmt.Fprintf(s, "%.2f %.2f m\n", cx+r, cy)
	fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx+r, cy+r*k, cx+r*k, cy+r, cx, cy+r)
	fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx-r*k, cy+r, cx-r, cy+r*k, cx-r, cy)
	fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx-r, cy-r*k, cx-r*k, cy-r, cx, cy-r)
	fmt.Fprintf(s, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx+r*k, cy-r, cx+r, cy-r*k, cx+r, cy)
	s.WriteString("h\n")
}

// selo: disco grafite com anel lime e um "check" branco.
func selo(s *bytes.Buffer, cx, cy, r float64) {
	fmt.Fprintf(s, "%s rg\n", corGrafite)
	circulo(s, cx, cy, r)
	s.WriteString("f\n")
	fmt.Fprintf(s, "%s RG %.2f w\n", corLime, r*0.12)
	circulo(s, cx, cy, r*0.80)
	s.WriteString("S\n")
	fmt.Fprintf(s, "%s RG %.2f w 1 J 1 j\n", corBranco, r*0.16)
	fmt.Fprintf(s, "%.2f %.2f m %.2f %.2f l %.2f %.2f l S\n",
		cx-r*0.36, cy+r*0.02, cx-r*0.10, cy-r*0.26, cx+r*0.38, cy+r*0.28)
}

func texto(s *bytes.Buffer, f fonte, tam, espacamento float64, cor string, x, y float64, t string) {
	fmt.Fprintf(s, "BT\n%s rg\n%s %.2f Tf\n%.3f Tc\n%.2f %.2f Td\n%s Tj\nET\n",
		cor, f.nome, tam, espacamento, x, y, pdfTexto(t))
}

// urlCurta tira o esquema para o link caber melhor ("lcr.d4ffy.com/validar").
func urlCurta(u string) string {
	return strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
}

// carimbo desenha o quadro visível como um Form XObject em PDF puro
// (Helvetica, WinAnsi), sem imagens nem fontes embutidas:
//
//	╭──────────────────────────────────────────╮
//	│ (✓)  DOCUMENTO ASSINADO DIGITALMENTE     │
//	│      NOME DO SIGNATÁRIO                  │
//	│      CPF ***.982.247-**                  │
//	│      15/11/2026 20:30:10 (UTC-03:00)     │
//	│▓▓ Verifique em lcr.d4ffy.com/validar ▓▓▓▓│
//	╰──────────────────────────────────────────╯
func carimbo(p port.PedidoAssinaturaPDF) func(*sign.SignContext, [4]float64) ([]byte, error) {
	return func(_ *sign.SignContext, r [4]float64) ([]byte, error) {
		w, h := r[2]-r[0], r[3]-r[1]
		if w < 40 || h < 20 {
			return nil, errors.New("área da assinatura muito pequena")
		}

		raio := math.Min(4, math.Min(w, h)*0.08)
		// Rodapé proporcional, mas limitado: em quadros altos ele não cresce demais.
		rodape := math.Min(math.Max(h*0.22, math.Min(h*0.30, 9)), 20)
		corpoH := h - rodape
		pad := math.Max(2, math.Min(corpoH*0.12, w*0.04))

		var s bytes.Buffer
		s.WriteString("q\n")
		// Fundo grafite inteiro (vira o rodapé) e o corpo branco por cima.
		fmt.Fprintf(&s, "%s rg\n", corGrafite)
		caminhoArredondado(&s, 0, 0, w, h, raio, false)
		s.WriteString("f\n")
		fmt.Fprintf(&s, "%s rg\n", corFundo)
		caminhoArredondado(&s, 0, rodape, w, corpoH, raio, true)
		s.WriteString("f\n")
		// Filete lime separando corpo e rodapé.
		fmt.Fprintf(&s, "%s rg\n0 %.2f %.2f %.2f re f\n", corLime, rodape, w, math.Max(0.8, h*0.018))
		// Contorno.
		fmt.Fprintf(&s, "%s RG 0.6 w\n", corBorda)
		caminhoArredondado(&s, 0.3, 0.3, w-0.6, h-0.6, raio, false)
		s.WriteString("S\n")

		// Selo à esquerda, centralizado no corpo.
		rs := math.Min((corpoH-2*pad)/2, w*0.11)
		xTexto := pad
		if rs >= 5 {
			selo(&s, pad+rs, rodape+corpoH/2, rs)
			xTexto = pad + 2*rs + pad
		}
		s.WriteString("Q\n")

		// Bloco de texto do corpo: dimensiona o corpo base para caber na
		// altura e na largura disponíveis.
		linhas := []linha{
			{"DOCUMENTO ASSINADO DIGITALMENTE", negrito, 0.70, corSutil, 0.06},
			{strings.ToUpper(p.Nome), negrito, 1.20, corGrafite, 0},
			{"CPF " + p.CPF.Mascarado(), regular, 0.86, corGrafite, 0},
			{p.Data.Format("02/01/2006 15:04:05") + " (UTC" + p.Data.Format("-07:00") + ")", regular, 0.86, corGrafite, 0},
		}
		if m := strings.TrimSpace(p.Motivo); m != "" {
			linhas = append(linhas, linha{"Motivo: " + m, regular, 0.78, corSutil, 0})
		}
		const entrelinha = 1.22
		dispW := w - xTexto - pad
		dispH := corpoH - 2*pad
		soma := 0.0
		for _, l := range linhas {
			soma += l.escala * entrelinha
		}
		base := dispH / soma
		for _, l := range linhas {
			if lw := l.fonte.largura(l.texto, base*l.escala, base*l.espacamento); lw > dispW {
				base *= dispW / lw
			}
		}
		base = math.Min(base, 16) // num quadro enorme, o texto não fica gigante

		alturaBloco := base * soma
		y := rodape + corpoH/2 + alturaBloco/2
		for _, l := range linhas {
			tam := base * l.escala
			y -= tam * entrelinha
			texto(&s, l.fonte, tam, base*l.espacamento, l.cor, xTexto, y+tam*0.24, l.texto)
		}

		// Rodapé: link de validação, centralizado. Se não couber, usa uma
		// versão mais curta da frase.
		url := urlCurta(p.URLValidar)
		dispRod := w - 2*pad
		for _, prefixo := range []string{"Verifique a autenticidade em ", "Verifique em ", ""} {
			tam := math.Min(rodape*0.50, base*0.86) // nunca maior que o texto do corpo
			lp := regular.largura(prefixo, tam, 0)
			lu := negrito.largura(url, tam, 0)
			if lp+lu > dispRod {
				tam *= dispRod / (lp + lu)
				lp, lu = regular.largura(prefixo, tam, 0), negrito.largura(url, tam, 0)
			}
			if tam < rodape*0.38 && prefixo != "" {
				continue // pequeno demais: tenta uma frase mais curta
			}
			x := (w - lp - lu) / 2
			yb := (rodape - tam*0.72) / 2
			if prefixo != "" {
				texto(&s, regular, tam, 0, corBranco, x, yb, prefixo)
			}
			texto(&s, negrito, tam, 0, corLime, x+lp, yb, url)
			break
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
