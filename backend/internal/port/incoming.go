// Package port define as fronteiras do hexágono.
//
//   - incoming.go: portas primárias (driving), por onde o mundo externo
//     (HTTP hoje; poderia ser CLI ou gRPC) aciona a aplicação.
//   - outgoing.go: portas secundárias (driven), por onde a aplicação usa a
//     infraestrutura (PostgreSQL, criptografia, AC X.509, biblioteca PAdES).
package port

import (
	"context"
	"time"

	"lcr/internal/domain"
)

// ContaService: cadastro, login e sessão.
type ContaService interface {
	// Cadastrar cria o usuário e já pede à AC o primeiro certificado.
	Cadastrar(ctx context.Context, nome, cpf, senha string) (*domain.Usuario, error)
	Autenticar(ctx context.Context, cpf, senha string) (*domain.Usuario, error)
	Usuario(ctx context.Context, id int64) (*domain.Usuario, error)

	IniciarSessao(ctx context.Context, usuarioID int64) (token string, expira time.Time, err error)
	UsuarioDaSessao(ctx context.Context, token string) (int64, error)
	EncerrarSessao(ctx context.Context, token string) error
}

// CertificadoService: ciclo de vida do certificado do titular.
type CertificadoService interface {
	Atual(ctx context.Context, usuarioID int64) (*domain.Certificado, *domain.DetalhesCertificado, error)
	ExportarPFX(ctx context.Context, usuarioID int64, senha string) ([]byte, error)
	Revogar(ctx context.Context, usuarioID int64, senha string, motivo domain.MotivoRevogacao) error
	Renovar(ctx context.Context, usuarioID int64, senha string) error
}

// ComandoAssinar reúne o que o titular envia para assinar.
type ComandoAssinar struct {
	UsuarioID   int64
	NomeArquivo string
	PDF         domain.PDF
	Posicao     domain.Posicao
	Senha       string
	Motivo      string
}

// AssinaturaService: assinar e consultar documentos assinados.
type AssinaturaService interface {
	Assinar(ctx context.Context, cmd ComandoAssinar) (*domain.Documento, error)
	Documentos(ctx context.Context, usuarioID int64) ([]domain.Documento, error)
	Documento(ctx context.Context, usuarioID, id int64) (*domain.Documento, error)
}

// ValidacaoService: validação pública, sem login.
type ValidacaoService interface {
	Validar(ctx context.Context, pdf domain.PDF) (*domain.ResultadoValidacao, error)
}

// ACService: informações públicas da Autoridade Certificadora.
type ACService interface {
	Info(ctx context.Context) (*domain.InfoAC, error)
	LCR(ctx context.Context) ([]byte, error)
	RaizPEM() []byte
	IntermediariaPEM() []byte
}
