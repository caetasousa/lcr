package port

import (
	"context"
	"time"

	"lcr/internal/domain"
)

// ---------- Persistência ----------

type UsuarioRepository interface {
	Criar(ctx context.Context, u *domain.Usuario) error // ErrCPFJaCadastrado se o CPF existe
	PorCPF(ctx context.Context, cpf domain.CPF) (*domain.Usuario, error)
	PorID(ctx context.Context, id int64) (*domain.Usuario, error)
}

type CertificadoRepository interface {
	Criar(ctx context.Context, c *domain.Certificado) error
	// Atual é o certificado mais recente do usuário (pode estar revogado).
	Atual(ctx context.Context, usuarioID int64) (*domain.Certificado, error)
	SalvarRevogacao(ctx context.Context, c *domain.Certificado) error
	Revogados(ctx context.Context) ([]domain.Revogacao, error)
}

type SessaoRepository interface {
	Criar(ctx context.Context, tokenHash []byte, usuarioID int64, expira time.Time) error
	UsuarioDe(ctx context.Context, tokenHash []byte, agora time.Time) (int64, error)
	Apagar(ctx context.Context, tokenHash []byte) error
}

type DocumentoRepository interface {
	Salvar(ctx context.Context, d *domain.Documento) error
	DoUsuario(ctx context.Context, usuarioID int64) ([]domain.Documento, error)
	Buscar(ctx context.Context, usuarioID, id int64) (*domain.Documento, error)
}

// ArmazemAC guarda o certificado e a chave (selada) de cada nível da AC.
type ArmazemAC interface {
	CarregarAC(ctx context.Context, nivel string) (certPEM, chaveSelada []byte, err error)
	SalvarAC(ctx context.Context, nivel string, certPEM, chaveSelada []byte) error
}

// ---------- Criptografia ----------

// HasherSenha produz e confere hashes de senha (Argon2id).
type HasherSenha interface {
	Gerar(senha string) (string, error)
	Conferir(senha, hash string) bool
}

// Cofre cifra segredos com uma chave derivada de senha (Argon2id + AES-GCM).
// Abrir devolve domain.ErrSenhaIncorreta quando a autenticação falha.
type Cofre interface {
	Selar(segredo []byte, senha, contexto string) ([]byte, error)
	Abrir(blob []byte, senha, contexto string) ([]byte, error)
}

// GeradorToken cria tokens aleatórios e o hash que vai para o banco.
type GeradorToken interface {
	Novo() (token string, hash []byte, err error)
	Hash(token string) []byte
}

// AutoridadeCertificadora é a AC ACME (raiz + intermediária).
type AutoridadeCertificadora interface {
	// Emitir gera o par de chaves do titular e o certificado assinado pela AC.
	Emitir(ctx context.Context, nome string, cpf domain.CPF) (*domain.Emissao, error)
	GerarLCR(ctx context.Context, revogados []domain.Revogacao) ([]byte, error)
	// ConsultarLCR confere a assinatura da LCR e procura o número de série.
	ConsultarLCR(lcr []byte, serial string) (domain.ConsultaLCR, error)
	Descrever(certPEM []byte) (*domain.DetalhesCertificado, error)
	ExportarPKCS12(certPEM, chavePKCS8 []byte, senha string) ([]byte, error)
	RaizPEM() []byte
	IntermediariaPEM() []byte
	CadeiaPEM() [][]byte // [intermediária, raiz]
	NomeEmissor() string // CN de quem emite os certificados finais
}

// PedidoAssinaturaPDF é tudo que o assinador PAdES precisa.
type PedidoAssinaturaPDF struct {
	PDF        domain.PDF
	CertPEM    []byte
	ChavePKCS8 []byte
	CadeiaPEM  [][]byte
	Nome       string
	CPF        domain.CPF
	Motivo     string
	Local      string
	Posicao    domain.Posicao
	Data       time.Time
	URLValidar string
}

type AssinadorPDF interface {
	Assinar(ctx context.Context, p PedidoAssinaturaPDF) ([]byte, error)
}

type ValidadorPDF interface {
	// Validar faz as verificações técnicas; a decisão final é do domínio.
	Validar(ctx context.Context, pdf domain.PDF) (*domain.ResultadoValidacao, error)
}
