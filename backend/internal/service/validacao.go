package service

import (
	"context"

	"lcr/internal/domain"
	"lcr/internal/port"
)

type ValidacaoDeps struct {
	Certificados port.CertificadoRepository
	AC           port.AutoridadeCertificadora
	Validador    port.ValidadorPDF
}

type Validacao struct {
	d ValidacaoDeps
}

var _ port.ValidacaoService = (*Validacao)(nil)

func NovaValidacao(d ValidacaoDeps) *Validacao { return &Validacao{d: d} }

// Validar combina as verificações técnicas (adaptador PAdES) com a consulta
// à LCR atual da AC e aplica a regra de negócio do domínio.
func (s *Validacao) Validar(ctx context.Context, pdf domain.PDF) (*domain.ResultadoValidacao, error) {
	res, err := s.d.Validador.Validar(ctx, pdf)
	if err != nil {
		return nil, err
	}
	res.SHA256 = sha256Hex(pdf)

	revogados, err := s.d.Certificados.Revogados(ctx)
	if err != nil {
		return nil, err
	}
	lcr, err := s.d.AC.GerarLCR(ctx, revogados)
	if err != nil {
		return nil, err
	}
	for i := range res.Assinaturas {
		a := &res.Assinaturas[i]
		var consulta domain.ConsultaLCR
		if a.Certificado != nil {
			a.EmitidoPorEstaAC = a.Certificado.Emissor == s.d.AC.NomeEmissor()
			if consulta, err = s.d.AC.ConsultarLCR(lcr, a.Certificado.Serial); err != nil {
				return nil, err
			}
		}
		a.Avaliar(consulta)
	}
	res.Consolidar()
	return res, nil
}
