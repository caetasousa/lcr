// Command carga é um teste de carga do LCR Assina: N usuários virtuais
// executam, AO MESMO TEMPO, cada etapa do uso real (cadastro, login, assinar,
// listar, baixar e validar) e o programa mede tempos e erros de cada etapa.
//
//	go run ./cmd/carga -url http://localhost:8080 -usuarios 60 -kb 500
//
// Use sempre numa instalação de TESTE: ele cria contas de verdade no banco.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type usuario struct {
	nome, cpf, senha string
	http             *http.Client
	docID            string
	assinado         []byte
}

type resultado struct {
	status int
	dur    time.Duration
	err    error
}

func main() {
	base := flag.String("url", "http://localhost:8080", "endereço do LCR Assina")
	n := flag.Int("usuarios", 60, "usuários simultâneos")
	kb := flag.Int("kb", 500, "tamanho aproximado do PDF, em KB")
	flag.Parse()

	pdf := gerarPDF(*kb * 1024)
	execucao := strconv.FormatInt(time.Now().Unix()%100000, 10)
	us := make([]*usuario, *n)
	for i := range us {
		jar, _ := cookiejar.New(nil)
		us[i] = &usuario{
			nome:  fmt.Sprintf("Carga %s %03d", execucao, i+1),
			cpf:   gerarCPF(),
			senha: "senha-de-carga-123",
			http: &http.Client{Jar: jar, Timeout: 3 * time.Minute, Transport: &http.Transport{
				MaxIdleConnsPerHost: 4, DisableCompression: true}},
		}
	}

	fmt.Printf("🧪 %d usuários simultâneos · PDF de %d KB · %s\n\n", *n, len(pdf)/1024, *base)
	fmt.Printf("%-10s %5s %6s %8s %8s %8s %8s  %s\n", "etapa", "ok", "erros", "p50", "p95", "máx", "total", "status")
	inicio := time.Now()
	falhou := false

	etapa := func(nome string, f func(u *usuario) (int, error), ok int) {
		res := make([]resultado, len(us))
		var wg sync.WaitGroup
		largada := make(chan struct{})
		t0 := time.Now()
		for i, u := range us {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-largada // todos partem juntos
				t := time.Now()
				st, err := f(u)
				res[i] = resultado{st, time.Since(t), err}
			}()
		}
		close(largada)
		wg.Wait()
		total := time.Since(t0)

		var durs []time.Duration
		porStatus := map[string]int{}
		oks := 0
		for _, r := range res {
			durs = append(durs, r.dur)
			chave := strconv.Itoa(r.status)
			if r.err != nil {
				chave = "falha de rede"
			}
			porStatus[chave]++
			if r.err == nil && r.status == ok {
				oks++
			}
		}
		slices.Sort(durs)
		pct := func(p float64) time.Duration { return durs[int(p*float64(len(durs)-1))] }
		var st []string
		for k, v := range porStatus {
			st = append(st, fmt.Sprintf("%s×%d", k, v))
		}
		slices.Sort(st)
		erros := len(us) - oks
		if erros > 0 {
			falhou = true
		}
		fmt.Printf("%-10s %5d %6d %8s %8s %8s %8s  %s\n", nome, oks, erros,
			arred(pct(0.5)), arred(pct(0.95)), arred(durs[len(durs)-1]), arred(total), strings.Join(st, " "))
	}

	etapa("cadastro", func(u *usuario) (int, error) {
		return postJSON(u, *base+"/api/cadastro", map[string]string{"nome": u.nome, "cpf": u.cpf, "senha": u.senha}, nil)
	}, http.StatusCreated)
	etapa("login", func(u *usuario) (int, error) {
		return postJSON(u, *base+"/api/login", map[string]string{"cpf": u.cpf, "senha": u.senha}, nil)
	}, http.StatusOK)
	etapa("assinar", func(u *usuario) (int, error) { return assinar(u, *base, pdf) }, http.StatusOK)
	etapa("listar", func(u *usuario) (int, error) {
		r, err := u.http.Get(*base + "/api/documentos")
		if err != nil {
			return 0, err
		}
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		return r.StatusCode, nil
	}, http.StatusOK)
	etapa("baixar", func(u *usuario) (int, error) {
		var corpo bytes.Buffer
		st, err := postJSON(u, *base+"/api/documentos/"+u.docID, map[string]string{"senha": u.senha}, &corpo)
		if err == nil && st == http.StatusOK && !bytes.Equal(corpo.Bytes(), u.assinado) {
			return -1, nil // -1 = arquivo diferente do assinado
		}
		return st, err
	}, http.StatusOK)
	etapa("validar", func(u *usuario) (int, error) {
		var corpo bytes.Buffer
		st, err := enviarPDF(u, *base+"/api/validar", u.assinado, nil, &corpo)
		if err == nil && st == http.StatusOK && !strings.Contains(corpo.String(), `"valido":true`) {
			return -2, nil // -2 = respondeu, mas a assinatura não validou
		}
		return st, err
	}, http.StatusOK)

	fmt.Printf("\n⏱️  tempo total: %s\n", arred(time.Since(inicio)))
	if falhou {
		os.Exit(1)
	}
}

func assinar(u *usuario, base string, pdf []byte) (int, error) {
	campos := map[string]string{"pagina": "1", "x1": "40", "y1": "40", "x2": "290", "y2": "106", "senha": u.senha, "motivo": "teste de carga"}
	var corpo bytes.Buffer
	r, err := enviar(u, base+"/api/assinar", pdf, campos, &corpo)
	if err != nil {
		return 0, err
	}
	if r.StatusCode == http.StatusOK {
		u.docID = r.Header.Get("X-Documento-Id")
		u.assinado = bytes.Clone(corpo.Bytes())
	}
	return r.StatusCode, nil
}

func enviarPDF(u *usuario, url string, pdf []byte, campos map[string]string, corpo *bytes.Buffer) (int, error) {
	r, err := enviar(u, url, pdf, campos, corpo)
	if err != nil {
		return 0, err
	}
	return r.StatusCode, nil
}

func enviar(u *usuario, url string, pdf []byte, campos map[string]string, corpo *bytes.Buffer) (*http.Response, error) {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, _ := mw.CreateFormFile("arquivo", "contrato.pdf")
	_, _ = fw.Write(pdf)
	for k, v := range campos {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, url, &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	_, err = io.Copy(corpo, r.Body)
	return r, err
}

func postJSON(u *usuario, url string, v any, corpo *bytes.Buffer) (int, error) {
	j, _ := json.Marshal(v)
	r, err := u.http.Post(url, "application/json", bytes.NewReader(j))
	if err != nil {
		return 0, err
	}
	defer r.Body.Close()
	if corpo == nil {
		corpo = &bytes.Buffer{}
	}
	_, err = io.Copy(corpo, r.Body)
	return r.StatusCode, err
}

// gerarPDF monta um PDF válido de uma página com um fluxo de conteúdo
// preenchido até o tamanho pedido (comentários PDF, que não aparecem).
func gerarPDF(tamanho int) []byte {
	var conteudo bytes.Buffer
	conteudo.WriteString("BT /F1 24 Tf 72 760 Td (Contrato de teste de carga) Tj ET\n")
	linha := "% " + strings.Repeat("x", 76) + "\n"
	for conteudo.Len() < tamanho {
		conteudo.WriteString(linha)
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", conteudo.Len(), conteudo.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// gerarCPF devolve um CPF aleatório com dígitos verificadores válidos.
func gerarCPF() string {
	d := make([]int, 0, 11)
	for range 9 {
		x, _ := rand.Int(rand.Reader, big.NewInt(10))
		d = append(d, int(x.Int64()))
	}
	for _, peso := range []int{10, 11} {
		s := 0
		for i, v := range d {
			s += v * (peso - i)
		}
		r := s * 10 % 11
		if r == 10 {
			r = 0
		}
		d = append(d, r)
	}
	var b strings.Builder
	for _, v := range d {
		b.WriteByte(byte('0' + v))
	}
	return b.String()
}

func arred(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(10 * time.Millisecond).String()
}
