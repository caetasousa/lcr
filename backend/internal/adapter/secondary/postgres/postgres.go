// Package postgres implementa as portas de persistência com pgx v5.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"lcr/internal/domain"
)

// Conectar abre o pool (com tentativas, pois no docker compose o banco pode
// ainda estar subindo) e aplica o schema.
func Conectar(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	cfg.MaxConns = 10 // limite explícito

	var pool *pgxpool.Pool
	for tentativa := 1; ; tentativa++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pctx)
			cancel()
			if err == nil {
				break
			}
			pool.Close()
		}
		if tentativa == 30 {
			return nil, fmt.Errorf("banco indisponível: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("aplicando schema: %w", err)
	}
	return pool, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS ac (
	tipo         TEXT PRIMARY KEY,           -- 'raiz' | 'intermediaria'
	cert_pem     TEXT NOT NULL,
	chave_selada BYTEA NOT NULL,             -- PKCS#8 cifrado (AES-256-GCM + Argon2id)
	criado_em    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS usuarios (
	id         BIGSERIAL PRIMARY KEY,
	nome       TEXT NOT NULL,
	cpf        TEXT NOT NULL UNIQUE,
	senha_hash TEXT NOT NULL,                -- Argon2id (formato PHC)
	criado_em  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS certificados (
	id               BIGSERIAL PRIMARY KEY,
	usuario_id       BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
	serial           TEXT NOT NULL UNIQUE,
	cert_pem         TEXT NOT NULL,
	chave_selada     BYTEA NOT NULL,         -- só abre com a senha do titular
	emitido_em       TIMESTAMPTZ NOT NULL,
	expira_em        TIMESTAMPTZ NOT NULL,
	revogado_em      TIMESTAMPTZ,
	motivo_revogacao INT
);
CREATE INDEX IF NOT EXISTS certificados_usuario ON certificados(usuario_id);
CREATE TABLE IF NOT EXISTS sessoes (
	token_hash BYTEA PRIMARY KEY,            -- SHA-256 do token do cookie
	usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
	expira_em  TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS documentos (
	id              BIGSERIAL PRIMARY KEY,
	usuario_id      BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
	certificado_id  BIGINT NOT NULL REFERENCES certificados(id),
	nome_arquivo    TEXT NOT NULL,
	sha256_original TEXT NOT NULL,
	sha256_assinado TEXT NOT NULL,
	pagina          INT NOT NULL,
	assinado_em     TIMESTAMPTZ NOT NULL,
	pdf_assinado    BYTEA NOT NULL
);
CREATE INDEX IF NOT EXISTS documentos_usuario ON documentos(usuario_id);
CREATE INDEX IF NOT EXISTS documentos_sha256_original ON documentos(sha256_original);
CREATE INDEX IF NOT EXISTS sessoes_expira ON sessoes(expira_em);
-- Desde a cifra híbrida, pdf_assinado guarda o PDF cifrado (AES-256-GCM).
-- cifrado=false marca documentos antigos, ainda em claro, a migrar.
ALTER TABLE documentos ADD COLUMN IF NOT EXISTS cifrado BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE documentos ADD COLUMN IF NOT EXISTS tamanho INT;
-- Uma linha por signatário: a chave AES do documento cifrada (RSA-OAEP) para
-- a chave pública do certificado. Ter uma linha aqui = ter acesso.
CREATE TABLE IF NOT EXISTS documento_chaves (
	documento_id   BIGINT NOT NULL REFERENCES documentos(id) ON DELETE CASCADE,
	certificado_id BIGINT NOT NULL REFERENCES certificados(id) ON DELETE CASCADE,
	chave_cifrada  BYTEA NOT NULL,
	PRIMARY KEY (documento_id, certificado_id)
);
CREATE INDEX IF NOT EXISTS documento_chaves_certificado ON documento_chaves(certificado_id);
`

// traduz converte erros do driver em erros do domínio.
func traduz(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNaoEncontrado
	}
	return err
}

func violacaoUnica(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
