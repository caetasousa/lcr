package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"lcr/internal/domain"
	"lcr/internal/port"
)

type AssinaturaDeps struct {
	Usuarios     port.UsuarioRepository
	Certificados port.CertificadoRepository
	Documentos   port.DocumentoRepository
	Cofre        port.Cofre
	AC           port.AutoridadeCertificadora
	Assinador    port.AssinadorPDF
	// Validador lê as assinaturas de dentro do PDF para descobrir todos os
	// signatários, e Cifrador cifra o arquivo para cada um deles.
	Validador port.ValidadorPDF
	Cifrador  port.CifradorDocumento
}

type Assinaturas struct {
	d   AssinaturaDeps
	cfg config
}

var _ port.AssinaturaService = (*Assinaturas)(nil)

func NovoAssinaturas(d AssinaturaDeps, opts ...Opcao) (*Assinaturas, error) {
	cfg, err := aplica(opts)
	if err != nil {
		return nil, err
	}
	return &Assinaturas{d: d, cfg: cfg}, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Assinaturas) Assinar(ctx context.Context, cmd port.ComandoAssinar) (*domain.Documento, error) {
	agora := s.cfg.agora()
	u, err := s.d.Usuarios.PorID(ctx, cmd.UsuarioID)
	if err != nil {
		return nil, err
	}
	cert, err := s.d.Certificados.Atual(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if err := cert.PodeAssinar(agora); err != nil {
		return nil, err
	}
	// A senha libera a chave privada só durante esta assinatura.
	chave, err := s.d.Cofre.Abrir(cert.ChaveSelada, cmd.Senha, contextoChaveUsuario)
	if err != nil {
		return nil, err
	}
	defer zera(chave)

	assinado, err := s.d.Assinador.Assinar(ctx, port.PedidoAssinaturaPDF{
		PDF: cmd.PDF, CertPEM: cert.CertPEM, ChavePKCS8: chave, CadeiaPEM: s.d.AC.CadeiaPEM(),
		Nome: u.Nome, CPF: u.CPF, Motivo: strings.TrimSpace(cmd.Motivo), Local: s.cfg.local,
		Posicao: cmd.Posicao, Data: agora, URLValidar: s.cfg.urlPublica + "/validar",
	})
	if err != nil {
		return nil, err
	}

	base := filepath.Base(cmd.NomeArquivo)
	doc := &domain.Documento{
		UsuarioID: u.ID, CertificadoID: cert.ID,
		NomeArquivo:    strings.TrimSuffix(base, filepath.Ext(base)) + "_assinado.pdf",
		SHA256Original: sha256Hex(cmd.PDF), SHA256Assinado: sha256Hex(assinado),
		Pagina: cmd.Posicao.Pagina, AssinadoEm: agora, PDFAssinado: assinado, Tamanho: len(assinado),
	}
	chaves, err := s.cifrarParaSignatarios(ctx, doc, assinado, cert)
	if err != nil {
		return nil, err
	}
	if err := s.d.Documentos.Salvar(ctx, doc, chaves); err != nil {
		return nil, err
	}
	return doc, nil
}

// cifrarParaSignatarios cifra o PDF e gera uma chave de acesso para cada
// pessoa que assinou: quem acabou de assinar e quem já tinha assinado antes
// (descobertos pelos certificados dentro do próprio PDF).
func (s *Assinaturas) cifrarParaSignatarios(ctx context.Context, doc *domain.Documento, pdf []byte, atual *domain.Certificado) ([]domain.ChaveDocumento, error) {
	certs := s.signatarios(ctx, pdf, atual)
	pems := make([][]byte, len(certs))
	for i, c := range certs {
		pems[i] = c.CertPEM
	}
	conteudo, cifradas, err := s.d.Cifrador.Cifrar(pdf, pems)
	if err != nil {
		return nil, fmt.Errorf("cifrando o documento: %w", err)
	}
	doc.Conteudo = conteudo
	chaves := make([]domain.ChaveDocumento, len(certs))
	for i, c := range certs {
		chaves[i] = domain.ChaveDocumento{CertificadoID: c.ID, ChaveCifrada: cifradas[i]}
	}
	return chaves, nil
}

// signatarios devolve os certificados (desta AC) de todas as assinaturas
// íntegras do PDF, sempre incluindo o certificado de quem está gravando.
func (s *Assinaturas) signatarios(ctx context.Context, pdf []byte, garantido *domain.Certificado) []*domain.Certificado {
	certs := []*domain.Certificado{garantido}
	vistos := map[int64]bool{garantido.ID: true}
	res, err := s.d.Validador.Validar(ctx, pdf)
	if err != nil {
		slog.WarnContext(ctx, "não foi possível ler os signatários do PDF", "err", err)
		return certs
	}
	for _, a := range res.Assinaturas {
		// Só assinaturas criptograficamente íntegras dão acesso.
		if !a.Integra || a.Certificado == nil {
			continue
		}
		c, err := s.d.Certificados.PorSerial(ctx, a.Certificado.Serial)
		if err != nil || vistos[c.ID] {
			continue
		}
		vistos[c.ID] = true
		certs = append(certs, c)
	}
	return certs
}

func (s *Assinaturas) Documentos(ctx context.Context, usuarioID int64) ([]domain.Documento, error) {
	return s.d.Documentos.DoUsuario(ctx, usuarioID)
}

// Documento: a senha abre a chave privada do usuário, que abre a chave do
// documento, que decifra o PDF. Antes de entregar, o SHA-256 precisa ser
// exatamente o registrado na assinatura (fidelidade ao original).
func (s *Assinaturas) Documento(ctx context.Context, usuarioID, id int64, senha string) (*domain.Documento, error) {
	doc, chave, err := s.d.Documentos.Buscar(ctx, usuarioID, id)
	if err != nil {
		return nil, err
	}
	cert, err := s.d.Certificados.PorID(ctx, chave.CertificadoID)
	if err != nil {
		return nil, err
	}
	priv, err := s.d.Cofre.Abrir(cert.ChaveSelada, senha, contextoChaveUsuario)
	if err != nil {
		return nil, err
	}
	defer zera(priv)
	pdf, err := s.d.Cifrador.Decifrar(doc.Conteudo, chave.ChaveCifrada, priv)
	if err != nil {
		return nil, err
	}
	if err := doc.ConfereIntegridade(pdf, sha256Hex); err != nil {
		return nil, err
	}
	doc.PDFAssinado = pdf
	return doc, nil
}

// MigrarLegados cifra os documentos que versões antigas gravaram em claro,
// dando acesso a todos os signatários. Roda na partida do servidor.
func (s *Assinaturas) MigrarLegados(ctx context.Context) (int, error) {
	docs, err := s.d.Documentos.Legados(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range docs {
		d := &docs[i]
		if err := d.ConfereIntegridade(d.Conteudo, sha256Hex); err != nil {
			// Não cifra o que já não confere: fica registrado para análise.
			slog.ErrorContext(ctx, "documento legado não confere com o SHA-256 gravado", "id", d.ID)
			continue
		}
		dono, err := s.d.Certificados.PorID(ctx, d.CertificadoID)
		if err != nil {
			return n, err
		}
		pdf := d.Conteudo
		chaves, err := s.cifrarParaSignatarios(ctx, d, pdf, dono)
		if err != nil {
			return n, err
		}
		if err := s.d.Documentos.CifrarLegado(ctx, d, chaves); err != nil {
			if errors.Is(err, domain.ErrNaoEncontrado) {
				continue // outra instância já migrou
			}
			return n, err
		}
		n++
	}
	return n, nil
}
