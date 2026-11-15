package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"lcr/internal/domain"
	"lcr/internal/port"
)

// ContaDeps agrupa as portas secundárias usadas pelo serviço de conta.
type ContaDeps struct {
	Usuarios     port.UsuarioRepository
	Certificados port.CertificadoRepository
	Sessoes      port.SessaoRepository
	Hasher       port.HasherSenha
	Cofre        port.Cofre
	Tokens       port.GeradorToken
	AC           port.AutoridadeCertificadora
}

type Conta struct {
	d   ContaDeps
	cfg config
	// hashFicticio equaliza o tempo de resposta quando o CPF não existe,
	// para o login não revelar quais CPFs têm conta.
	hashFicticio string
}

var _ port.ContaService = (*Conta)(nil)

func NovaConta(d ContaDeps, opts ...Opcao) (*Conta, error) {
	cfg, err := aplica(opts)
	if err != nil {
		return nil, err
	}
	h, err := d.Hasher.Gerar("senha-ficticia-para-tempo-constante")
	if err != nil {
		return nil, err
	}
	return &Conta{d: d, cfg: cfg, hashFicticio: h}, nil
}

func (s *Conta) Cadastrar(ctx context.Context, nome, cpfTexto, senha string) (*domain.Usuario, error) {
	nome, err := domain.NormalizaNome(nome)
	if err != nil {
		return nil, err
	}
	cpf, err := domain.NovoCPF(cpfTexto)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidaSenhaNova(senha); err != nil {
		return nil, err
	}
	if _, err := s.d.Usuarios.PorCPF(ctx, cpf); err == nil {
		return nil, domain.ErrCPFJaCadastrado
	} else if !errors.Is(err, domain.ErrNaoEncontrado) {
		return nil, err
	}

	hash, err := s.d.Hasher.Gerar(senha)
	if err != nil {
		return nil, fmt.Errorf("gerando hash da senha: %w", err)
	}
	// A AC emite antes de gravar o usuário: se a emissão falhar, não sobra
	// conta sem certificado.
	cert, err := emitir(ctx, s.d.AC, s.d.Cofre, nome, cpf, senha)
	if err != nil {
		return nil, err
	}
	u := &domain.Usuario{Nome: nome, CPF: cpf, SenhaHash: hash}
	if err := s.d.Usuarios.Criar(ctx, u); err != nil {
		return nil, err
	}
	cert.UsuarioID = u.ID
	if err := s.d.Certificados.Criar(ctx, cert); err != nil {
		return nil, fmt.Errorf("gravando certificado: %w", err)
	}
	return u, nil
}

func (s *Conta) Autenticar(ctx context.Context, cpfTexto, senha string) (*domain.Usuario, error) {
	cpf, err := domain.NovoCPF(cpfTexto)
	if err != nil {
		return nil, domain.ErrCredenciaisInvalidas
	}
	u, err := s.d.Usuarios.PorCPF(ctx, cpf)
	if errors.Is(err, domain.ErrNaoEncontrado) {
		s.d.Hasher.Conferir(senha, s.hashFicticio)
		return nil, domain.ErrCredenciaisInvalidas
	}
	if err != nil {
		return nil, err
	}
	if !s.d.Hasher.Conferir(senha, u.SenhaHash) {
		return nil, domain.ErrCredenciaisInvalidas
	}
	return u, nil
}

func (s *Conta) Usuario(ctx context.Context, id int64) (*domain.Usuario, error) {
	return s.d.Usuarios.PorID(ctx, id)
}

func (s *Conta) IniciarSessao(ctx context.Context, usuarioID int64) (string, time.Time, error) {
	token, hash, err := s.d.Tokens.Novo()
	if err != nil {
		return "", time.Time{}, err
	}
	expira := s.cfg.agora().Add(s.cfg.duracaoSessao)
	if err := s.d.Sessoes.Criar(ctx, hash, usuarioID, expira); err != nil {
		return "", time.Time{}, err
	}
	return token, expira, nil
}

func (s *Conta) UsuarioDaSessao(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, domain.ErrSessaoInvalida
	}
	id, err := s.d.Sessoes.UsuarioDe(ctx, s.d.Tokens.Hash(token), s.cfg.agora())
	if errors.Is(err, domain.ErrNaoEncontrado) {
		return 0, domain.ErrSessaoInvalida
	}
	return id, err
}

func (s *Conta) EncerrarSessao(ctx context.Context, token string) error {
	return s.d.Sessoes.Apagar(ctx, s.d.Tokens.Hash(token))
}

// emitir pede o certificado à AC e sela a chave privada com a senha do
// titular. A chave em claro só existe em memória durante esta função.
func emitir(ctx context.Context, ac port.AutoridadeCertificadora, cofre port.Cofre, nome string, cpf domain.CPF, senha string) (*domain.Certificado, error) {
	e, err := ac.Emitir(ctx, nome, cpf)
	if err != nil {
		return nil, fmt.Errorf("emitindo certificado: %w", err)
	}
	defer zera(e.ChavePKCS8)
	selada, err := cofre.Selar(e.ChavePKCS8, senha, contextoChaveUsuario)
	if err != nil {
		return nil, fmt.Errorf("selando chave privada: %w", err)
	}
	return &domain.Certificado{
		Serial: e.Serial, CertPEM: e.CertPEM, ChaveSelada: selada,
		EmitidoEm: e.ValidoDe, ExpiraEm: e.ValidoAte,
	}, nil
}

// zera sobrescreve material de chave depois do uso (melhor esforço: o GC
// pode ter feito cópias, mas reduz o tempo de exposição).
func zera(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
