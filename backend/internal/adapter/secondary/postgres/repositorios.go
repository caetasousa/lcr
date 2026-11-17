package postgres

import (
	"context"
	"errors"
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

const colunasCertificado = `id, usuario_id, serial, cert_pem, chave_selada, emitido_em, expira_em, revogado_em, motivo_revogacao`

func (r *Certificados) um(ctx context.Context, where string, arg any) (*domain.Certificado, error) {
	c := &domain.Certificado{}
	var pemTxt string
	var motivo *int
	err := r.db.QueryRow(ctx, `SELECT `+colunasCertificado+` FROM certificados WHERE `+where, arg).
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

func (r *Certificados) Atual(ctx context.Context, usuarioID int64) (*domain.Certificado, error) {
	return r.um(ctx, "usuario_id=$1 ORDER BY id DESC LIMIT 1", usuarioID)
}

func (r *Certificados) PorID(ctx context.Context, id int64) (*domain.Certificado, error) {
	return r.um(ctx, "id=$1", id)
}

func (r *Certificados) PorSerial(ctx context.Context, serial string) (*domain.Certificado, error) {
	return r.um(ctx, "serial=$1", serial)
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
	// Aproveita para apagar as sessões vencidas: a tabela não cresce sem
	// limite mesmo que ninguém clique em "sair".
	if _, err := r.db.Exec(ctx, `DELETE FROM sessoes WHERE expira_em < now()`); err != nil {
		return err
	}
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

// Salvar grava o documento cifrado e as chaves de acesso numa transação:
// nunca existe documento sem ao menos um signatário capaz de abri-lo.
func (r *Documentos) Salvar(ctx context.Context, d *domain.Documento, chaves []domain.ChaveDocumento) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO documentos (usuario_id, certificado_id, nome_arquivo, sha256_original, sha256_assinado, pagina, assinado_em, pdf_assinado, tamanho, cifrado)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,true) RETURNING id`,
			d.UsuarioID, d.CertificadoID, d.NomeArquivo, d.SHA256Original, d.SHA256Assinado, d.Pagina, d.AssinadoEm, d.Conteudo, d.Tamanho).Scan(&d.ID)
		if err != nil {
			return err
		}
		return inserirChaves(ctx, tx, d.ID, chaves)
	})
}

func inserirChaves(ctx context.Context, tx pgx.Tx, docID int64, chaves []domain.ChaveDocumento) error {
	if len(chaves) == 0 {
		return errors.New("documento sem chave de acesso")
	}
	for _, k := range chaves {
		if _, err := tx.Exec(ctx,
			`INSERT INTO documento_chaves (documento_id, certificado_id, chave_cifrada) VALUES ($1,$2,$3)
			 ON CONFLICT (documento_id, certificado_id) DO NOTHING`,
			docID, k.CertificadoID, k.ChaveCifrada); err != nil {
			return err
		}
	}
	return nil
}

func (r *Documentos) DoUsuario(ctx context.Context, usuarioID int64) ([]domain.Documento, error) {
	rows, err := r.db.Query(ctx,
		`SELECT d.id, d.nome_arquivo, d.sha256_original, d.sha256_assinado, d.pagina, d.assinado_em,
		        COALESCE(d.tamanho, octet_length(d.pdf_assinado)),
		        ARRAY(SELECT DISTINCT u.nome FROM documento_chaves k
		              JOIN certificados c ON c.id = k.certificado_id
		              JOIN usuarios u ON u.id = c.usuario_id
		              WHERE k.documento_id = d.id ORDER BY u.nome)
		 FROM documentos d
		 WHERE d.cifrado AND EXISTS (
		       SELECT 1 FROM documento_chaves k JOIN certificados c ON c.id = k.certificado_id
		       WHERE k.documento_id = d.id AND c.usuario_id = $1)
		   -- Cada assinatura grava uma versão nova do PDF. Esconde as versões
		   -- que já foram assinadas de novo (a nova partiu deste PDF) quando o
		   -- usuário também tem acesso à nova: assim ele valida/baixa sempre o
		   -- arquivo com TODAS as assinaturas, não uma cópia antiga com só uma.
		   AND NOT EXISTS (
		       SELECT 1 FROM documentos n
		       JOIN documento_chaves k2 ON k2.documento_id = n.id
		       JOIN certificados c2 ON c2.id = k2.certificado_id
		       WHERE n.cifrado AND n.sha256_original = d.sha256_assinado AND c2.usuario_id = $1)
		 ORDER BY d.id DESC LIMIT 500`, usuarioID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Documento, error) {
		var d domain.Documento
		err := row.Scan(&d.ID, &d.NomeArquivo, &d.SHA256Original, &d.SHA256Assinado, &d.Pagina, &d.AssinadoEm, &d.Tamanho, &d.Signatarios)
		return d, err
	})
}

func (r *Documentos) Buscar(ctx context.Context, usuarioID, id int64) (*domain.Documento, *domain.ChaveDocumento, error) {
	d := &domain.Documento{}
	k := &domain.ChaveDocumento{}
	err := r.db.QueryRow(ctx,
		`SELECT d.id, d.nome_arquivo, d.sha256_assinado, d.pdf_assinado, k.certificado_id, k.chave_cifrada
		 FROM documentos d
		 JOIN documento_chaves k ON k.documento_id = d.id
		 JOIN certificados c ON c.id = k.certificado_id
		 WHERE d.id = $1 AND c.usuario_id = $2 AND d.cifrado
		 ORDER BY c.id DESC LIMIT 1`, id, usuarioID).
		Scan(&d.ID, &d.NomeArquivo, &d.SHA256Assinado, &d.Conteudo, &k.CertificadoID, &k.ChaveCifrada)
	if err != nil {
		return nil, nil, traduz(err)
	}
	return d, k, nil
}

func (r *Documentos) Legados(ctx context.Context) ([]domain.Documento, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, usuario_id, certificado_id, sha256_assinado, pdf_assinado FROM documentos WHERE NOT cifrado ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Documento, error) {
		var d domain.Documento
		err := row.Scan(&d.ID, &d.UsuarioID, &d.CertificadoID, &d.SHA256Assinado, &d.Conteudo)
		return d, err
	})
}

// CifrarLegado troca o PDF em claro pelo cifrado e grava as chaves, na mesma
// transação. Se o documento já tiver sido migrado, devolve ErrNaoEncontrado.
func (r *Documentos) CifrarLegado(ctx context.Context, d *domain.Documento, chaves []domain.ChaveDocumento) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE documentos SET pdf_assinado=$2, tamanho=octet_length(pdf_assinado), cifrado=true WHERE id=$1 AND NOT cifrado`,
			d.ID, d.Conteudo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNaoEncontrado
		}
		return inserirChaves(ctx, tx, d.ID, chaves)
	})
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
