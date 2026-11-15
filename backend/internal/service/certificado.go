package service

import (
	"context"

	"lcr/internal/domain"
	"lcr/internal/port"
)

type CertificadoDeps struct {
	Usuarios     port.UsuarioRepository
	Certificados port.CertificadoRepository
	Hasher       port.HasherSenha
	Cofre        port.Cofre
	AC           port.AutoridadeCertificadora
}

type Certificados struct {
	d   CertificadoDeps
	cfg config
}

var _ port.CertificadoService = (*Certificados)(nil)

func NovoCertificados(d CertificadoDeps, opts ...Opcao) (*Certificados, error) {
	cfg, err := aplica(opts)
	if err != nil {
		return nil, err
	}
	return &Certificados{d: d, cfg: cfg}, nil
}

func (s *Certificados) Atual(ctx context.Context, usuarioID int64) (*domain.Certificado, *domain.DetalhesCertificado, error) {
	c, err := s.d.Certificados.Atual(ctx, usuarioID)
	if err != nil {
		return nil, nil, err
	}
	det, err := s.d.AC.Descrever(c.CertPEM)
	if err != nil {
		return nil, nil, err
	}
	return c, det, nil
}

func (s *Certificados) ExportarPFX(ctx context.Context, usuarioID int64, senha string) ([]byte, error) {
	c, err := s.d.Certificados.Atual(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := c.PodeAssinar(s.cfg.agora()); err != nil {
		return nil, err
	}
	chave, err := s.d.Cofre.Abrir(c.ChaveSelada, senha, contextoChaveUsuario)
	if err != nil {
		return nil, err
	}
	defer zera(chave)
	return s.d.AC.ExportarPKCS12(c.CertPEM, chave, senha)
}

func (s *Certificados) confereSenha(ctx context.Context, usuarioID int64, senha string) (*domain.Usuario, error) {
	u, err := s.d.Usuarios.PorID(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if !s.d.Hasher.Conferir(senha, u.SenhaHash) {
		return nil, domain.ErrSenhaIncorreta
	}
	return u, nil
}

func (s *Certificados) Revogar(ctx context.Context, usuarioID int64, senha string, motivo domain.MotivoRevogacao) error {
	if !motivo.Valido() {
		return domain.ErrMotivoInvalido
	}
	if _, err := s.confereSenha(ctx, usuarioID, senha); err != nil {
		return err
	}
	c, err := s.d.Certificados.Atual(ctx, usuarioID)
	if err != nil {
		return err
	}
	if err := c.Revogar(s.cfg.agora(), motivo); err != nil {
		return err
	}
	return s.d.Certificados.SalvarRevogacao(ctx, c)
}

func (s *Certificados) Renovar(ctx context.Context, usuarioID int64, senha string) error {
	u, err := s.confereSenha(ctx, usuarioID, senha)
	if err != nil {
		return err
	}
	atual, err := s.d.Certificados.Atual(ctx, usuarioID)
	if err != nil {
		return err
	}
	if err := atual.PodeSerSubstituido(s.cfg.agora()); err != nil {
		return err
	}
	novo, err := emitir(ctx, s.d.AC, s.d.Cofre, u.Nome, u.CPF, senha)
	if err != nil {
		return err
	}
	novo.UsuarioID = u.ID
	return s.d.Certificados.Criar(ctx, novo)
}
