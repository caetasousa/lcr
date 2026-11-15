package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lcr/internal/domain"
	"lcr/internal/port"
)

// ---------- Usuários ----------

type Usuarios struct{ db *pgxpool.Pool }

var _ port.UsuarioRepository = (*Usuarios)(nil)

func NovoUsuarios(db *pgxpool.Pool) *Usuarios { return &Usuarios{db: db} }

func (r *Usuarios) Criar(ctx context.Context, u *domain.Usuario) error {
	err := r.db.QueryRow(ctx,
		`INSERT INTO usuarios (nome, cpf, senha_hash) VALUES ($1,$2,$3) RETURNING id, criado_em`,
		u.Nome, string(u.CPF), u.SenhaHash).Scan(&u.ID, &u.CriadoEm)
	if violacaoUnica(err) {
		return domain.ErrCPFJaCadastrado
	}
	return err
}

func (r *Usuarios) busca(ctx context.Context, where string, arg any) (*domain.Usuario, error) {
	u := &domain.Usuario{}
	var cpf string
	err := r.db.QueryRow(ctx, `SELECT id, nome, cpf, senha_hash, criado_em FROM usuarios WHERE `+where, arg).
		Scan(&u.ID, &u.Nome, &cpf, &u.SenhaHash, &u.CriadoEm)
	u.CPF = domain.CPF(cpf)
	return u, traduz(err)
}

func (r *Usuarios) PorCPF(ctx context.Context, cpf domain.CPF) (*domain.Usuario, error) {
	return r.busca(ctx, "cpf=$1", string(cpf))
}

func (r *Usuarios) PorID(ctx context.Context, id int64) (*domain.Usuario, error) {
	return r.busca(ctx, "id=$1", id)
}

// ---------- Certificados ----------

type Certificados struct{ db *pgxpool.Pool }

var _ port.CertificadoRepository = (*Certificados)(nil)

func NovoCertificados(db *pgxpool.Pool) *Certificados { return &Certificados{db: db} }

func (r *Certificados) Criar(ctx context.Context, c *domain.Certificado) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO certificados (usuario_id, serial, cert_pem, chave_selada, emitido_em, expira_em)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		c.UsuarioID, c.Serial, string(c.CertPEM), c.ChaveSelada, c.EmitidoEm, c.ExpiraEm).Scan(&c.ID)
}

func (r *Certificados) Atual(ctx context.Context, usuarioID int64) (*domain.Certificado, error) {
	c := &domain.Certificado{}
	var pemTxt string
	var motivo *int
	err := r.db.QueryRow(ctx,
		`SELECT id, usuario_id, serial, cert_pem, chave_selada, emitido_em, expira_em, revogado_em, motivo_revogacao
		 FROM certificados WHERE usuario_id=$1 ORDER BY id DESC LIMIT 1`, usuarioID).
		Scan(&c.ID, &c.UsuarioID, &c.Serial, &pemTxt, &c.ChaveSelada, &c.EmitidoEm, &c.ExpiraEm, &c.RevogadoEm, &motivo)
	if err != nil {
		return nil, traduz(err)
	}
	c.CertPEM = []byte(pemTxt)
	if motivo != nil {
		m := domain.MotivoRevogacao(*motivo)
		c.Motivo = &m
	}
	return c, nil
}

func (r *Certificados) SalvarRevogacao(ctx context.Context, c *domain.Certificado) error {
	if c.RevogadoEm == nil || c.Motivo == nil {
		return domain.ErrMotivoInvalido
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE certificados SET revogado_em=$2, motivo_revogacao=$3 WHERE id=$1 AND revogado_em IS NULL`,
		c.ID, *c.RevogadoEm, int(*c.Motivo))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrCertificadoJaRevogado
	}
	return nil
}

func (r *Certificados) Revogados(ctx context.Context) ([]domain.Revogacao, error) {
	rows, err := r.db.Query(ctx,
		`SELECT c.serial, u.nome, c.revogado_em, COALESCE(c.motivo_revogacao, 0)
		 FROM certificados c JOIN usuarios u ON u.id = c.usuario_id
		 WHERE c.revogado_em IS NOT NULL ORDER BY c.revogado_em`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Revogacao, error) {
		var v domain.Revogacao
		var m int
		err := row.Scan(&v.Serial, &v.Titular, &v.Em, &m)
		v.Motivo = domain.MotivoRevogacao(m)
		return v, err
	})
}

// ---------- Sessões ----------

type Sessoes struct{ db *pgxpool.Pool }

var _ port.SessaoRepository = (*Sessoes)(nil)

func NovoSessoes(db *pgxpool.Pool) *Sessoes { return &Sessoes{db: db} }

func (r *Sessoes) Criar(ctx context.Context, hash []byte, usuarioID int64, expira time.Time) error {
	_, err := r.db.Exec(ctx, `INSERT INTO sessoes (token_hash, usuario_id, expira_em) VALUES ($1,$2,$3)`, hash, usuarioID, expira)
	return err
}

func (r *Sessoes) UsuarioDe(ctx context.Context, hash []byte, agora time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `SELECT usuario_id FROM sessoes WHERE token_hash=$1 AND expira_em > $2`, hash, agora).Scan(&id)
	return id, traduz(err)
}

// Apagar remove a sessão e aproveita para limpar as expiradas.
func (r *Sessoes) Apagar(ctx context.Context, hash []byte) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessoes WHERE token_hash=$1 OR expira_em < now()`, hash)
	return err
}

// ---------- Documentos ----------

type Documentos struct{ db *pgxpool.Pool }

var _ port.DocumentoRepository = (*Documentos)(nil)

func NovoDocumentos(db *pgxpool.Pool) *Documentos { return &Documentos{db: db} }

func (r *Documentos) Salvar(ctx context.Context, d *domain.Documento) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO documentos (usuario_id, certificado_id, nome_arquivo, sha256_original, sha256_assinado, pagina, assinado_em, pdf_assinado)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		d.UsuarioID, d.CertificadoID, d.NomeArquivo, d.SHA256Original, d.SHA256Assinado, d.Pagina, d.AssinadoEm, d.PDFAssinado).Scan(&d.ID)
}

func (r *Documentos) DoUsuario(ctx context.Context, usuarioID int64) ([]domain.Documento, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, nome_arquivo, sha256_original, sha256_assinado, pagina, assinado_em, octet_length(pdf_assinado)
		 FROM documentos WHERE usuario_id=$1 ORDER BY id DESC LIMIT 500`, usuarioID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Documento, error) {
		var d domain.Documento
		err := row.Scan(&d.ID, &d.NomeArquivo, &d.SHA256Original, &d.SHA256Assinado, &d.Pagina, &d.AssinadoEm, &d.Tamanho)
		return d, err
	})
}

func (r *Documentos) Buscar(ctx context.Context, usuarioID, id int64) (*domain.Documento, error) {
	d := &domain.Documento{}
	err := r.db.QueryRow(ctx,
		`SELECT id, nome_arquivo, pdf_assinado FROM documentos WHERE id=$1 AND usuario_id=$2`, id, usuarioID).
		Scan(&d.ID, &d.NomeArquivo, &d.PDFAssinado)
	return d, traduz(err)
}

// ---------- Armazém da AC ----------

type ArmazemAC struct{ db *pgxpool.Pool }

var _ port.ArmazemAC = (*ArmazemAC)(nil)

func NovoArmazemAC(db *pgxpool.Pool) *ArmazemAC { return &ArmazemAC{db: db} }

func (r *ArmazemAC) CarregarAC(ctx context.Context, nivel string) ([]byte, []byte, error) {
	var certPEM string
	var chave []byte
	err := r.db.QueryRow(ctx, `SELECT cert_pem, chave_selada FROM ac WHERE tipo=$1`, nivel).Scan(&certPEM, &chave)
	return []byte(certPEM), chave, traduz(err)
}

func (r *ArmazemAC) SalvarAC(ctx context.Context, nivel string, certPEM, chave []byte) error {
	_, err := r.db.Exec(ctx, `INSERT INTO ac (tipo, cert_pem, chave_selada) VALUES ($1,$2,$3)`, nivel, string(certPEM), chave)
	return err
}
