package service

import (
	"context"

	"lcr/internal/domain"
	"lcr/internal/port"
)

type AC struct {
	certs port.CertificadoRepository
	ac    port.AutoridadeCertificadora
}

var _ port.ACService = (*AC)(nil)

func NovaAC(certs port.CertificadoRepository, ac port.AutoridadeCertificadora) *AC {
	return &AC{certs: certs, ac: ac}
}

func (s *AC) Info(ctx context.Context) (*domain.InfoAC, error) {
	raiz, err := s.ac.Descrever(s.ac.RaizPEM())
	if err != nil {
		return nil, err
	}
	inter, err := s.ac.Descrever(s.ac.IntermediariaPEM())
	if err != nil {
		return nil, err
	}
	revs, err := s.certs.Revogados(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.InfoAC{Raiz: *raiz, Intermediaria: *inter, Revogados: revs}, nil
}

func (s *AC) LCR(ctx context.Context) ([]byte, error) {
	revs, err := s.certs.Revogados(ctx)
	if err != nil {
		return nil, err
	}
	return s.ac.GerarLCR(ctx, revs)
}

func (s *AC) RaizPEM() []byte          { return s.ac.RaizPEM() }
func (s *AC) IntermediariaPEM() []byte { return s.ac.IntermediariaPEM() }
