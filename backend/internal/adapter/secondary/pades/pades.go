// Package pades implementa AssinadorPDF e ValidadorPDF com a biblioteca
// digitorus/pdfsign: assinatura CMS (CAdES) embutida no PDF por atualização
// incremental, ou seja, PAdES-B-B.
package pades

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/sign"
	"github.com/digitorus/pdfsign/verify"

	"lcr/internal/domain"
	"lcr/internal/port"
)

// ---------------------------------------------------------------------------
// Assinador
// ---------------------------------------------------------------------------

type Assinador struct{}

var _ port.AssinadorPDF = Assinador{}

// Assinar devolve o PDF com a assinatura embutida. Por dentro:
//  1. o PDF original é copiado sem alterações;
//  2. é anexada uma atualização incremental com o campo de assinatura, o
//     carimbo visual e um espaço reservado (/Contents);
//  3. calcula-se o SHA-256 de todo o arquivo EXCETO esse espaço (/ByteRange);
//  4. o hash é assinado com RSA dentro de um envelope CMS, que leva junto o
//     certificado e a cadeia;
//  5. o CMS é gravado em hexadecimal no /Contents.
func (Assinador) Assinar(ctx context.Context, p port.PedidoAssinaturaPDF) ([]byte, error) {
	cert, err := certDePEM(p.CertPEM)
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKCS8PrivateKey(p.ChavePKCS8)
	if err != nil {
		return nil, fmt.Errorf("chave privada inválida: %w", err)
	}
	chave, ok := k.(crypto.Signer)
	if !ok {
		return nil, errors.New("chave privada não suporta assinatura")
	}
	cadeia := []*x509.Certificate{cert}
	for _, c := range p.CadeiaPEM {
		x, err := certDePEM(c)
		if err != nil {
			return nil, err
		}
		cadeia = append(cadeia, x)
	}

	rd := bytes.NewReader(p.PDF)
	leitor, err := pdf.NewReader(rd, int64(len(p.PDF)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrPDFInvalido, err)
	}
	if n := leitor.NumPage(); p.Posicao.Pagina > n {
		return nil, fmt.Errorf("%w: a página %d não existe (o PDF tem %d)", domain.ErrPosicaoInvalida, p.Posicao.Pagina, n)
	}

	r := p.Posicao.Retangulo
	var saida bytes.Buffer
	err = sign.SignWithData(rd, &saida, leitor, int64(len(p.PDF)), sign.SignData{
		Context: ctx,
		Signature: sign.SignDataSignature{
			Info: sign.SignDataSignatureInfo{
				Name: p.Nome, Location: p.Local, Reason: p.Motivo,
				ContactInfo: "CPF " + p.CPF.Mascarado(), Date: p.Data,
			},
			// "Approval" permite várias assinaturas no mesmo documento.
			CertType: sign.ApprovalSignature,
		},
		Signer:            chave,
		DigestAlgorithm:   crypto.SHA256,
		Certificate:       cert,
		CertificateChains: [][]*x509.Certificate{cadeia},
		SubFilter:         sign.SubFilterETSICAdESDetached, // PAdES
		Appearance: sign.Appearance{
			Visible: true, Page: uint32(p.Posicao.Pagina),
			LowerLeftX: r.X1, LowerLeftY: r.Y1, UpperRightX: r.X2, UpperRightY: r.Y2,
			Renderer: carimbo(p),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("falha ao assinar: %w", err)
	}
	return saida.Bytes(), nil
}

func certDePEM(b []byte) (*x509.Certificate, error) {
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, errors.New("PEM de certificado inválido")
	}
	return x509.ParseCertificate(blk.Bytes)
}

// ---------------------------------------------------------------------------
// Validador
// ---------------------------------------------------------------------------

// Validador confia apenas na raiz informada (a AC ACME Raiz G2).
type Validador struct{ raizes *x509.CertPool }

var _ port.ValidadorPDF = (*Validador)(nil)

func NovoValidador(raizPEM []byte) (*Validador, error) {
	raiz, err := certDePEM(raizPEM)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(raiz)
	return &Validador{raizes: pool}, nil
}

func (v *Validador) Validar(ctx context.Context, doc domain.PDF) (*domain.ResultadoValidacao, error) {
	opts := verify.DefaultVerifyOptions()
	opts.TrustedRoots = v.raizes
	opts.Context = ctx
	// Sem carimbo de tempo de uma ACT, a única data disponível é a declarada
	// na assinatura. Aceitamos para fins didáticos.
	opts.TrustSignatureTime = true
	opts.RequireNonRepudiation = true

	resp, err := verify.VerifyWithOptions(bytes.NewReader(doc), int64(len(doc)), opts)
	res := &domain.ResultadoValidacao{Assinaturas: []domain.ResultadoAssinatura{}}
	if err != nil {
		res.Erro = err.Error()
	}
	if resp == nil {
		return res, nil
	}
	res.Paginas = resp.DocumentInfo.Pages
	if resp.Error != "" && res.Erro == "" {
		res.Erro = resp.Error
	}
	for _, s := range resp.Signers {
		a := domain.ResultadoAssinatura{
			Nome: s.Name, Motivo: s.Reason, Local: s.Location, Contato: s.ContactInfo,
			DataAssinatura: s.SignatureTime, Integra: s.ValidSignature,
			CadeiaConfiavel: s.TrustedIssuer, RevogadoNoPDF: s.RevokedCertificate,
			Cadeia: []domain.CertInfo{}, Erros: []string{}, Avisos: []string{},
		}
		for i, c := range s.Certificates {
			if c.Certificate == nil {
				continue
			}
			ci := domain.CertInfo{
				Titular: c.Certificate.Subject.CommonName, Emissor: c.Certificate.Issuer.CommonName,
				Serial:   strings.ToUpper(fmt.Sprintf("%X", c.Certificate.SerialNumber.Bytes())),
				ValidoDe: c.Certificate.NotBefore, ValidoAte: c.Certificate.NotAfter,
			}
			if i == 0 { // a biblioteca coloca o signatário primeiro
				a.Certificado = &ci
			}
			a.Cadeia = append(a.Cadeia, ci)
		}
		for _, e := range s.ValidationErrors {
			a.Erros = append(a.Erros, e.Error())
		}
		for _, w := range s.Warnings {
			a.Avisos = append(a.Avisos, w.Error())
		}
		res.Assinaturas = append(res.Assinaturas, a)
	}
	return res, nil
}
