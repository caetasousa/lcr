// Package acx509 implementa a porta AutoridadeCertificadora com crypto/x509:
//
//	AC ACME Raiz G2 (autoassinada, RSA-4096, 10 anos, chave "offline")
//	 └── AC ACME Assinatura Digital (RSA-3072, 5 anos: emite usuários e a LCR)
//	      └── NOME:CPF (RSA-3072, 1 ano)
package acx509

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"lcr/internal/domain"
	"lcr/internal/port"
)

// OIDDocumentSigning é o EKU "Document Signing" (RFC 9336). O Go não tem
// constante para ele, então vai em UnknownExtKeyUsage.
var OIDDocumentSigning = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 36}

const (
	nivelRaiz          = "raiz"
	nivelIntermediaria = "intermediaria"
	contextoAC         = "chave-ac:"
	CNRaiz             = "AC ACME Raiz G2"
	CNIntermediaria    = "AC ACME Assinatura Digital"
)

type config struct {
	urlPublica        string
	bitsRaiz          int
	bitsIntermediaria int
	bitsUsuario       int
	agora             func() time.Time
}

// Opcao configura a AC (functional options).
type Opcao func(*config) error

// ComURLPublica define a base das URLs de LCR e AIA gravadas nos certificados.
func ComURLPublica(u string) Opcao {
	return func(c *config) error {
		p, err := url.Parse(u)
		if err != nil || p.Host == "" {
			return fmt.Errorf("URL pública inválida: %q", u)
		}
		c.urlPublica = strings.TrimRight(u, "/")
		return nil
	}
}

// ComTamanhosDeChave permite chaves menores em testes (mínimo 2048).
func ComTamanhosDeChave(raiz, intermediaria, usuario int) Opcao {
	return func(c *config) error {
		for _, b := range []int{raiz, intermediaria, usuario} {
			if b < 2048 {
				return errors.New("chaves RSA menores que 2048 bits não são aceitas")
			}
		}
		c.bitsRaiz, c.bitsIntermediaria, c.bitsUsuario = raiz, intermediaria, usuario
		return nil
	}
}

func ComRelogio(agora func() time.Time) Opcao {
	return func(c *config) error { c.agora = agora; return nil }
}

// AC é a autoridade certificadora carregada em memória. A chave da raiz
// NÃO fica aqui: ela só é usada uma vez, para assinar a intermediária.
type AC struct {
	cfg        config
	raiz       *x509.Certificate
	inter      *x509.Certificate
	chaveInter crypto.Signer
}

var _ port.AutoridadeCertificadora = (*AC)(nil)

// Carregar abre a AC gravada no armazém ou, na primeira execução, cria a
// hierarquia e grava as chaves seladas com senhaAC.
func Carregar(ctx context.Context, armazem port.ArmazemAC, cofre port.Cofre, senhaAC string, opts ...Opcao) (*AC, error) {
	cfg := config{urlPublica: "http://localhost:8080", bitsRaiz: 4096, bitsIntermediaria: 3072, bitsUsuario: 3072, agora: time.Now}
	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}
	if len(senhaAC) < 12 {
		return nil, errors.New("a senha da AC deve ter pelo menos 12 caracteres")
	}

	raizPEM, _, errR := armazem.CarregarAC(ctx, nivelRaiz)
	interPEM, interSelada, errI := armazem.CarregarAC(ctx, nivelIntermediaria)
	switch {
	case errors.Is(errR, domain.ErrNaoEncontrado) && errors.Is(errI, domain.ErrNaoEncontrado):
		return criar(ctx, cfg, armazem, cofre, senhaAC)
	case errR != nil:
		return nil, errR
	case errI != nil:
		return nil, errI
	}

	raiz, err := parsePEM(raizPEM)
	if err != nil {
		return nil, err
	}
	inter, err := parsePEM(interPEM)
	if err != nil {
		return nil, err
	}
	der, err := cofre.Abrir(interSelada, senhaAC, contextoAC+nivelIntermediaria)
	if err != nil {
		return nil, errors.New("AC_SENHA não abre a chave da AC intermediária (a senha mudou?)")
	}
	chave, err := parseChave(der)
	if err != nil {
		return nil, err
	}
	return &AC{cfg: cfg, raiz: raiz, inter: inter, chaveInter: chave}, nil
}

func criar(ctx context.Context, cfg config, armazem port.ArmazemAC, cofre port.Cofre, senha string) (*AC, error) {
	slog.InfoContext(ctx, "primeira execução: criando a hierarquia da AC ACME")
	agora := cfg.agora()
	chaveRaiz, err := rsa.GenerateKey(rand.Reader, cfg.bitsRaiz)
	if err != nil {
		return nil, err
	}
	nome := func(cn string) pkix.Name {
		return pkix.Name{Country: []string{"BR"}, Organization: []string{"ACME"},
			OrganizationalUnit: []string{"Seguranca da Informacao"}, CommonName: cn}
	}
	tplRaiz := &x509.Certificate{
		Subject: nome(CNRaiz), NotBefore: agora.Add(-time.Minute), NotAfter: agora.AddDate(10, 0, 0),
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, BasicConstraintsValid: true, IsCA: true, MaxPathLen: 1,
	}
	raiz, err := assina(tplRaiz, tplRaiz, &chaveRaiz.PublicKey, chaveRaiz)
	if err != nil {
		return nil, err
	}
	chaveInter, err := rsa.GenerateKey(rand.Reader, cfg.bitsIntermediaria)
	if err != nil {
		return nil, err
	}
	tplInter := &x509.Certificate{
		Subject: nome(CNIntermediaria), NotBefore: agora.Add(-time.Minute), NotAfter: agora.AddDate(5, 0, 0),
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, BasicConstraintsValid: true, IsCA: true,
		MaxPathLen: 0, MaxPathLenZero: true, // só emite certificados finais
	}
	inter, err := assina(tplInter, raiz, &chaveInter.PublicKey, chaveRaiz)
	if err != nil {
		return nil, err
	}
	for _, n := range []struct {
		nivel string
		cert  *x509.Certificate
		chave crypto.Signer
	}{{nivelRaiz, raiz, chaveRaiz}, {nivelIntermediaria, inter, chaveInter}} {
		der, err := x509.MarshalPKCS8PrivateKey(n.chave)
		if err != nil {
			return nil, err
		}
		selada, err := cofre.Selar(der, senha, contextoAC+n.nivel)
		if err != nil {
			return nil, err
		}
		if err := armazem.SalvarAC(ctx, n.nivel, certPEM(n.cert), selada); err != nil {
			return nil, err
		}
	}
	return &AC{cfg: cfg, raiz: raiz, inter: inter, chaveInter: chaveInter}, nil
}

func assina(tpl, pai *x509.Certificate, pub crypto.PublicKey, chave crypto.Signer) (*x509.Certificate, error) {
	serial, err := novoSerial()
	if err != nil {
		return nil, err
	}
	tpl.SerialNumber = serial
	der, err := x509.CreateCertificate(rand.Reader, tpl, pai, pub, chave)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

// novoSerial: 128 bits aleatórios (o CA/B Forum exige >= 64 bits de entropia).
func novoSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func SerialHex(s *big.Int) string { return strings.ToUpper(hex.EncodeToString(s.Bytes())) }

func certPEM(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}

func parsePEM(b []byte) (*x509.Certificate, error) {
	blk, _ := pem.Decode(b)
	if blk == nil || blk.Type != "CERTIFICATE" {
		return nil, errors.New("PEM de certificado inválido")
	}
	return x509.ParseCertificate(blk.Bytes)
}

func parseChave(der []byte) (crypto.Signer, error) {
	k, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	s, ok := k.(crypto.Signer)
	if !ok {
		return nil, errors.New("chave não suporta assinatura")
	}
	return s, nil
}

// Emitir faz o papel da AC ao receber uma solicitação: gera o par de chaves
// do titular e assina a chave pública junto com a identidade. CN no padrão
// ICP-Brasil "NOME:CPF".
func (a *AC) Emitir(ctx context.Context, nome string, cpf domain.CPF) (*domain.Emissao, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	chave, err := rsa.GenerateKey(rand.Reader, a.cfg.bitsUsuario)
	if err != nil {
		return nil, err
	}
	agora := a.cfg.agora()
	tpl := &x509.Certificate{
		Subject: pkix.Name{
			Country: []string{"BR"}, Organization: []string{"ACME"},
			OrganizationalUnit: []string{"Colaboradores"},
			CommonName:         strings.ToUpper(nome) + ":" + string(cpf),
		},
		NotBefore: agora.Add(-time.Minute),
		NotAfter:  agora.AddDate(1, 0, 0), // 1 ano, como um A1
		// Digital Signature + Non Repudiation (ContentCommitment).
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection, x509.ExtKeyUsageClientAuth},
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{OIDDocumentSigning},
		BasicConstraintsValid: true,
		CRLDistributionPoints: []string{a.cfg.urlPublica + "/api/ac/lcr.crl"},
		IssuingCertificateURL: []string{a.cfg.urlPublica + "/api/ac/intermediaria.crt"},
	}
	cert, err := assina(tpl, a.inter, &chave.PublicKey, a.chaveInter)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(chave)
	if err != nil {
		return nil, err
	}
	return &domain.Emissao{
		Serial: SerialHex(cert.SerialNumber), CertPEM: certPEM(cert), ChavePKCS8: der,
		ValidoDe: cert.NotBefore, ValidoAte: cert.NotAfter,
	}, nil
}

// GerarLCR produz a LCR (DER) assinada pela AC intermediária.
func (a *AC) GerarLCR(_ context.Context, revogados []domain.Revogacao) ([]byte, error) {
	agora := a.cfg.agora()
	lista := &x509.RevocationList{
		Number:     big.NewInt(agora.UnixNano()), // sempre crescente
		ThisUpdate: agora,
		NextUpdate: agora.Add(24 * time.Hour),
	}
	for _, r := range revogados {
		n, ok := new(big.Int).SetString(r.Serial, 16)
		if !ok {
			return nil, fmt.Errorf("número de série inválido na base: %q", r.Serial)
		}
		lista.RevokedCertificateEntries = append(lista.RevokedCertificateEntries, x509.RevocationListEntry{
			SerialNumber: n, RevocationTime: r.Em, ReasonCode: int(r.Motivo),
		})
	}
	return x509.CreateRevocationList(rand.Reader, lista, a.inter, a.chaveInter)
}

// ConsultarLCR confere a assinatura da LCR com a AC intermediária e procura
// o número de série, como faria qualquer validador externo.
func (a *AC) ConsultarLCR(der []byte, serial string) (domain.ConsultaLCR, error) {
	lcr, err := x509.ParseRevocationList(der)
	if err != nil {
		return domain.ConsultaLCR{}, err
	}
	if err := lcr.CheckSignatureFrom(a.inter); err != nil {
		return domain.ConsultaLCR{}, fmt.Errorf("assinatura da LCR inválida: %w", err)
	}
	c := domain.ConsultaLCR{NumeroLCR: lcr.Number.String()}
	for _, e := range lcr.RevokedCertificateEntries {
		if SerialHex(e.SerialNumber) == serial {
			c.Revogado, c.RevogadoEm, c.Motivo = true, e.RevocationTime, domain.MotivoRevogacao(e.ReasonCode)
		}
	}
	return c, nil
}

// Descrever traduz um X.509 para o formato de exibição do domínio.
func (a *AC) Descrever(pemBytes []byte) (*domain.DetalhesCertificado, error) {
	x, err := parsePEM(pemBytes)
	if err != nil {
		return nil, err
	}
	bits := 0
	if pk, ok := x.PublicKey.(*rsa.PublicKey); ok {
		bits = pk.N.BitLen()
	}
	var usos []string
	if x.IsCA {
		usos = append(usos, "Certificate Sign", "CRL Sign")
	}
	if x.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		usos = append(usos, "Digital Signature")
	}
	if x.KeyUsage&x509.KeyUsageContentCommitment != 0 {
		usos = append(usos, "Non Repudiation")
	}
	for _, e := range x.ExtKeyUsage {
		switch e {
		case x509.ExtKeyUsageEmailProtection:
			usos = append(usos, "E-mail Protection")
		case x509.ExtKeyUsageClientAuth:
			usos = append(usos, "Client Authentication")
		}
	}
	for _, o := range x.UnknownExtKeyUsage {
		if o.Equal(OIDDocumentSigning) {
			usos = append(usos, "Document Signing")
		}
	}
	h := sha256.Sum256(x.Raw)
	fp := make([]string, len(h))
	for i, b := range h {
		fp[i] = fmt.Sprintf("%02X", b)
	}
	return &domain.DetalhesCertificado{
		Titular: x.Subject.String(), Emissor: x.Issuer.String(), Serial: SerialHex(x.SerialNumber),
		ValidoDe: x.NotBefore, ValidoAte: x.NotAfter,
		Algoritmo:    x.SignatureAlgorithm.String() + " / " + x.PublicKeyAlgorithm.String(),
		TamanhoChave: bits, Fingerprint: strings.Join(fp, ":"), Usos: usos,
		LCR: x.CRLDistributionPoints, PEM: string(pemBytes),
	}, nil
}

// ExportarPKCS12 gera um .pfx moderno (AES-256-CBC + PBKDF2-SHA256),
// aceito pelo Windows 10 1709+ e pelo OpenSSL 3.
func (a *AC) ExportarPKCS12(certPEMBytes, chavePKCS8 []byte, senha string) ([]byte, error) {
	cert, err := parsePEM(certPEMBytes)
	if err != nil {
		return nil, err
	}
	chave, err := parseChave(chavePKCS8)
	if err != nil {
		return nil, err
	}
	return pkcs12.Modern2023.Encode(chave, cert, []*x509.Certificate{a.inter, a.raiz}, senha)
}

func (a *AC) RaizPEM() []byte          { return certPEM(a.raiz) }
func (a *AC) IntermediariaPEM() []byte { return certPEM(a.inter) }
func (a *AC) CadeiaPEM() [][]byte      { return [][]byte{certPEM(a.inter), certPEM(a.raiz)} }
func (a *AC) NomeEmissor() string      { return a.inter.Subject.CommonName }

// FingerprintRaiz para log de inicialização.
func (a *AC) FingerprintRaiz() string {
	h := sha256.Sum256(a.raiz.Raw)
	return strings.ToUpper(hex.EncodeToString(h[:]))
}
