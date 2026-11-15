package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	if err := s.d.Documentos.Salvar(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *Assinaturas) Documentos(ctx context.Context, usuarioID int64) ([]domain.Documento, error) {
	return s.d.Documentos.DoUsuario(ctx, usuarioID)
}

func (s *Assinaturas) Documento(ctx context.Context, usuarioID, id int64) (*domain.Documento, error) {
	return s.d.Documentos.Buscar(ctx, usuarioID, id)
}
