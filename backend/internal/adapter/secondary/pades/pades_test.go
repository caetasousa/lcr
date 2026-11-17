package pades_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"lcr/internal/adapter/secondary/acx509"
	"lcr/internal/adapter/secondary/cripto"
	"lcr/internal/adapter/secondary/pades"
	"lcr/internal/domain"
	"lcr/internal/port"
)

type armazemMem map[string][2][]byte

func (a armazemMem) CarregarAC(_ context.Context, n string) ([]byte, []byte, error) {
	v, ok := a[n]
	if !ok {
		return nil, nil, domain.ErrNaoEncontrado
	}
	return v[0], v[1], nil
}

func (a armazemMem) SalvarAC(_ context.Context, n string, c, k []byte) error {
	a[n] = [2][]byte{c, k}
	return nil
}

// pdfMinimo gera um PDF válido de duas páginas.
func pdfMinimo() []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
	}
	for i := range 2 {
		conteudo := fmt.Sprintf("BT /F1 24 Tf 72 720 Td (Contrato ACME - pagina %d) Tj ET", i+1)
		objs = append(objs,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >> /Contents %d 0 R >>", 4+i*2),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(conteudo), conteudo))
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

func TestAssinarEValidarDeVerdade(t *testing.T) {
	if testing.Short() {
		t.Skip("gera chaves RSA")
	}
	ctx := context.Background()
	cofre, _ := cripto.NovoArgon2(cripto.Parametros{Tempo: 1, Memoria: 8 * 1024, Threads: 1, TamChave: 32})
	ac, err := acx509.Carregar(ctx, armazemMem{}, cofre, "senha-da-ac-teste",
		acx509.ComTamanhosDeChave(2048, 2048, 2048))
	if err != nil {
		t.Fatal(err)
	}
	cpf, _ := domain.NovoCPF("529.982.247-25")
	em, err := ac.Emitir(ctx, "Maria da Conceição", cpf)
	if err != nil {
		t.Fatal(err)
	}
	pos, _ := domain.NovaPosicao(2, 40, 40, 300, 110)
	original, _ := domain.NovoPDF(pdfMinimo())

	assinado, err := pades.Assinador{}.Assinar(ctx, port.PedidoAssinaturaPDF{
		PDF: original, CertPEM: em.CertPEM, ChavePKCS8: em.ChavePKCS8, CadeiaPEM: ac.CadeiaPEM(),
		Nome: "Maria da Conceição", CPF: cpf, Posicao: pos, Data: time.Now(), URLValidar: "http://localhost/validar",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(assinado, original) {
		t.Error("PAdES deve preservar os bytes originais (atualização incremental)")
	}

	v, err := pades.NovoValidador(ac.RaizPEM())
	if err != nil {
		t.Fatal(err)
	}
	res, err := v.Validar(ctx, assinado)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Assinaturas) != 1 {
		t.Fatalf("esperava 1 assinatura, veio %d (%s)", len(res.Assinaturas), res.Erro)
	}
	a := res.Assinaturas[0]
	if !a.Integra || !a.CadeiaConfiavel || len(a.Erros) > 0 {
		t.Fatalf("assinatura deveria ser íntegra e confiável: %+v", a)
	}
	if a.Certificado == nil || a.Certificado.Serial != em.Serial {
		t.Errorf("serial do signatário: %+v", a.Certificado)
	}

	// Muda 1 bit do conteúdo original: a integridade tem de falhar.
	alterado := bytes.Clone(assinado)
	i := bytes.Index(alterado, []byte("pagina 1"))
	alterado[i] ^= 1
	res2, _ := v.Validar(ctx, alterado)
	if len(res2.Assinaturas) == 1 && res2.Assinaturas[0].Integra {
		t.Error("documento alterado passou como íntegro")
	}

	// Página inexistente vira erro de domínio.
	pos5, _ := domain.NovaPosicao(5, 40, 40, 300, 110)
	_, err = pades.Assinador{}.Assinar(ctx, port.PedidoAssinaturaPDF{
		PDF: original, CertPEM: em.CertPEM, ChavePKCS8: em.ChavePKCS8, CadeiaPEM: ac.CadeiaPEM(), Posicao: pos5, Data: time.Now(),
	})
	if err == nil {
		t.Error("página 5 de um PDF de 2 páginas deveria falhar")
	}
}

// Dois signatários no mesmo PDF: a validação tem de listar os dois.
func TestDuasAssinaturas(t *testing.T) {
	if testing.Short() {
		t.Skip("gera chaves RSA")
	}
	ctx := context.Background()
	cofre, _ := cripto.NovoArgon2(cripto.Parametros{Tempo: 1, Memoria: 8 * 1024, Threads: 1, TamChave: 32})
	ac, err := acx509.Carregar(ctx, armazemMem{}, cofre, "senha-da-ac-teste",
		acx509.ComTamanhosDeChave(2048, 2048, 2048))
	if err != nil {
		t.Fatal(err)
	}
	cpf1, _ := domain.NovoCPF("529.982.247-25")
	cpf2, _ := domain.NovoCPF("111.444.777-35")
	em1, err := ac.Emitir(ctx, "Maria da Conceição", cpf1)
	if err != nil {
		t.Fatal(err)
	}
	em2, err := ac.Emitir(ctx, "João da Silva", cpf2)
	if err != nil {
		t.Fatal(err)
	}
	pdf, _ := domain.NovoPDF(pdfMinimo())
	pos1, _ := domain.NovaPosicao(2, 40, 40, 300, 110)
	pos2, _ := domain.NovaPosicao(2, 40, 200, 300, 270)

	assinar := func(pdf []byte, em *domain.Emissao, nome string, cpf domain.CPF, pos domain.Posicao) []byte {
		t.Helper()
		out, err := pades.Assinador{}.Assinar(ctx, port.PedidoAssinaturaPDF{
			PDF: pdf, CertPEM: em.CertPEM, ChavePKCS8: em.ChavePKCS8, CadeiaPEM: ac.CadeiaPEM(),
			Nome: nome, CPF: cpf, Posicao: pos, Data: time.Now(), URLValidar: "http://localhost/validar",
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	um := assinar(pdf, em1, "Maria da Conceição", cpf1, pos1)
	dois := assinar(um, em2, "João da Silva", cpf2, pos2)

	v, err := pades.NovoValidador(ac.RaizPEM())
	if err != nil {
		t.Fatal(err)
	}
	res, err := v.Validar(ctx, dois)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Assinaturas) != 2 {
		t.Fatalf("esperava 2 assinaturas, veio %d (%s)", len(res.Assinaturas), res.Erro)
	}
	seriais := map[string]bool{}
	for _, a := range res.Assinaturas {
		if !a.Integra || a.Certificado == nil {
			t.Errorf("assinatura deveria ser íntegra: %+v", a)
			continue
		}
		seriais[a.Certificado.Serial] = true
	}
	if !seriais[em1.Serial] || !seriais[em2.Serial] {
		t.Errorf("faltou signatário: %v (esperava %s e %s)", seriais, em1.Serial, em2.Serial)
	}
}

func TestLCR(t *testing.T) {
	ctx := context.Background()
	cofre, _ := cripto.NovoArgon2(cripto.Parametros{Tempo: 1, Memoria: 8 * 1024, Threads: 1, TamChave: 32})
	ac, err := acx509.Carregar(ctx, armazemMem{}, cofre, "senha-da-ac-teste", acx509.ComTamanhosDeChave(2048, 2048, 2048))
	if err != nil {
		t.Fatal(err)
	}
	em, _ := ac.Emitir(ctx, "X", "52998224725")
	quando := time.Now().Add(-time.Minute).Truncate(time.Second)
	lcr, err := ac.GerarLCR(ctx, []domain.Revogacao{{Serial: em.Serial, Em: quando, Motivo: domain.MotivoChaveComprometida}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := ac.ConsultarLCR(lcr, em.Serial)
	if err != nil || !c.Revogado || !c.RevogadoEm.Equal(quando) || c.Motivo != domain.MotivoChaveComprometida {
		t.Errorf("consulta: %+v, %v", c, err)
	}
	if c, _ := ac.ConsultarLCR(lcr, "ABCDEF"); c.Revogado {
		t.Error("serial desconhecido não pode constar como revogado")
	}
	lcr[len(lcr)-5] ^= 0xFF
	if _, err := ac.ConsultarLCR(lcr, em.Serial); err == nil {
		t.Error("LCR adulterada deveria ser rejeitada")
	}
}
