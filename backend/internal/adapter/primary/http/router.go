// Package httpadapter é o adaptador primário HTTP: traduz requisições em
// chamadas às portas primárias e erros do domínio em status HTTP. Também
// serve o frontend React (SPA).
package httpadapter

import (
	"context"
	"errors"
	"net/http"

	"lcr/internal/port"
)

// Servicos são as portas primárias que este adaptador aciona.
type Servicos struct {
	Conta        port.ContaService
	Certificados port.CertificadoService
	Assinaturas  port.AssinaturaService
	Validacao    port.ValidacaoService
	AC           port.ACService
}

type config struct {
	diretorioEstatico string
	cookieSeguro      bool
	maxPDF            int64
}

// Opcao configura o adaptador (functional options).
type Opcao func(*config) error

// ComDiretorioEstatico serve o build do React a partir de dir.
func ComDiretorioEstatico(dir string) Opcao {
	return func(c *config) error { c.diretorioEstatico = dir; return nil }
}

// ComCookieSeguro marca o cookie de sessão como Secure (exige HTTPS).
func ComCookieSeguro(v bool) Opcao {
	return func(c *config) error { c.cookieSeguro = v; return nil }
}

// ComTamanhoMaximoPDF limita o upload.
func ComTamanhoMaximoPDF(bytes int64) Opcao {
	return func(c *config) error {
		if bytes < 1<<20 {
			return errors.New("tamanho máximo de PDF deve ser pelo menos 1 MiB")
		}
		c.maxPDF = bytes
		return nil
	}
}

type Handler struct {
	s   Servicos
	cfg config
}

// Novo monta o roteador com todas as rotas.
func Novo(s Servicos, opts ...Opcao) (http.Handler, error) {
	cfg := config{maxPDF: 20 << 20}
	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}
	h := &Handler{s: s, cfg: cfg}
	m := http.NewServeMux()

	// Públicas
	m.HandleFunc("POST /api/cadastro", h.cadastro)
	m.HandleFunc("POST /api/login", h.login)
	m.HandleFunc("POST /api/logout", h.logout)
	m.HandleFunc("POST /api/validar", h.validar)
	m.HandleFunc("GET /api/ac", h.infoAC)
	m.HandleFunc("GET /api/ac/raiz.crt", h.baixaRaiz)
	m.HandleFunc("GET /api/ac/intermediaria.crt", h.baixaIntermediaria)
	m.HandleFunc("GET /api/ac/lcr.crl", h.baixaLCR)

	// Autenticadas
	m.Handle("GET /api/eu", h.autenticado(h.eu))
	m.Handle("POST /api/assinar", h.autenticado(h.assinar))
	m.Handle("GET /api/documentos", h.autenticado(h.documentos))
	// POST: o corpo leva a senha, que libera a chave privada para decifrar.
	m.Handle("POST /api/documentos/{id}", h.autenticado(h.baixaDocumento))
	m.Handle("GET /api/certificado.crt", h.autenticado(h.baixaCertificado))
	m.Handle("POST /api/certificado/pfx", h.autenticado(h.exportaPFX))
	m.Handle("POST /api/certificado/revogar", h.autenticado(h.revogar))
	m.Handle("POST /api/certificado/renovar", h.autenticado(h.renovar))

	m.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { erroJSON(w, http.StatusNotFound, "rota não encontrada") })
	m.HandleFunc("/", h.spa)
	return cabecalhosSeguranca(m), nil
}

func cabecalhosSeguranca(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// ---------- sessão ----------

const cookieSessao = "sessao"

type chaveCtx struct{}

func usuarioID(r *http.Request) int64 { return r.Context().Value(chaveCtx{}).(int64) }

func (h *Handler) autenticado(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieSessao)
		if err != nil {
			erroJSON(w, http.StatusUnauthorized, "faça login")
			return
		}
		id, err := h.s.Conta.UsuarioDaSessao(r.Context(), c.Value)
		if err != nil {
			responderErro(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), chaveCtx{}, id)))
	})
}
