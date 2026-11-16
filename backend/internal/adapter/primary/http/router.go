// Package httpadapter é o adaptador primário HTTP: traduz requisições em
// chamadas às portas primárias e erros do domínio em status HTTP. Também
// serve o frontend React (SPA).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"time"

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
	limitePDF         int           // requisições com PDF processadas ao mesmo tempo
	esperaMaxima      time.Duration // quanto uma requisição espera na fila antes do 503
	cabecalhoIP       string        // cabeçalho do proxy confiável com o IP real ("" = usar a conexão)
	semLimiteDeTaxa   bool          // desliga os limites por IP/usuário (só para teste de carga)
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

// ComLimiteDePDFs limita quantas requisições que carregam um PDF inteiro na
// memória (assinar, validar, baixar, exportar) rodam ao mesmo tempo. As
// demais esperam na fila até espera; depois disso recebem 503.
func ComLimiteDePDFs(n int, espera time.Duration) Opcao {
	return func(c *config) error {
		if n < 1 || espera <= 0 {
			return errors.New("limite de PDFs simultâneos deve ser >= 1 e a espera > 0")
		}
		c.limitePDF, c.esperaMaxima = n, espera
		return nil
	}
}

// ComCabecalhoDeIP informa em qual cabeçalho o proxy confiável entrega o IP
// real do cliente (ex.: "CF-Connecting-IP" atrás da Cloudflare). Só use
// quando a aplicação NÃO é alcançável sem passar por esse proxy; caso
// contrário o cliente poderia forjar o cabeçalho e burlar os limites.
func ComCabecalhoDeIP(nome string) Opcao {
	return func(c *config) error { c.cabecalhoIP = http.CanonicalHeaderKey(nome); return nil }
}

// SemLimiteDeTaxa desliga os limites de tentativas. Existe para o teste de
// carga, em que centenas de usuários virtuais saem do mesmo IP.
func SemLimiteDeTaxa() Opcao {
	return func(c *config) error { c.semLimiteDeTaxa = true; return nil }
}

type Handler struct {
	s     Servicos
	cfg   config
	vagas chan struct{}
	lim   *limites // nil = limites desligados
}

// Novo monta o roteador com todas as rotas.
func Novo(s Servicos, opts ...Opcao) (http.Handler, error) {
	cfg := config{maxPDF: 20 << 20, limitePDF: 4, esperaMaxima: 30 * time.Second}
	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}
	h := &Handler{s: s, cfg: cfg, vagas: make(chan struct{}, cfg.limitePDF)}
	if !cfg.semLimiteDeTaxa {
		h.lim = limitesPadrao()
	}
	m := http.NewServeMux()

	// Públicas
	m.HandleFunc("POST /api/cadastro", h.porIP(func(l *limites) *limitador { return l.cadastro }, "cadastro", h.cadastro))
	m.HandleFunc("POST /api/login", h.porIP(func(l *limites) *limitador { return l.login }, "login", h.login))
	m.HandleFunc("POST /api/logout", h.logout)
	m.Handle("POST /api/validar", h.porIP(func(l *limites) *limitador { return l.validar }, "validar", h.pesado(h.validar)))
	m.HandleFunc("GET /api/ac", h.infoAC)
	m.HandleFunc("GET /api/ac/raiz.crt", h.baixaRaiz)
	m.HandleFunc("GET /api/ac/intermediaria.crt", h.baixaIntermediaria)
	m.HandleFunc("GET /api/ac/lcr.crl", h.baixaLCR)

	// Autenticadas
	m.Handle("GET /api/eu", h.autenticado(h.eu))
	m.Handle("POST /api/assinar", h.autenticado(h.protegeSenha("assinatura", h.pesado(h.assinar))))
	m.Handle("GET /api/documentos", h.autenticado(h.documentos))
	// POST: o corpo leva a senha, que libera a chave privada para decifrar.
	m.Handle("POST /api/documentos/{id}", h.autenticado(h.protegeSenha("download_documento", h.pesado(h.baixaDocumento))))
	m.Handle("GET /api/certificado.crt", h.autenticado(h.baixaCertificado))
	m.Handle("POST /api/certificado/pfx", h.autenticado(h.protegeSenha("exportacao_pfx", h.pesado(h.exportaPFX))))
	m.Handle("POST /api/certificado/revogar", h.autenticado(h.protegeSenha("revogacao", h.revogar)))
	m.Handle("POST /api/certificado/renovar", h.autenticado(h.protegeSenha("renovacao", h.renovar)))

	m.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { erroJSON(w, http.StatusNotFound, "rota não encontrada") })
	m.HandleFunc("/", h.spa)
	// Ordem (de fora para dentro): panic -> cabeçalhos -> limite por IP ->
	// conferência de origem -> rotas.
	return h.recuperar(h.cabecalhosSeguranca(h.taxaGeral(h.mesmaOrigem(m)))), nil
}

// ---------- sessão ----------

// nomeCookie: em HTTPS o cookie usa o prefixo __Host-, que o navegador só
// aceita com Secure, Path=/ e SEM Domain. Assim outro app em um subdomínio
// irmão (*.d4ffy.com) não consegue plantar um cookie de sessão para cá.
func (h *Handler) nomeCookie() string {
	if h.cfg.cookieSeguro {
		return "__Host-sessao"
	}
	return "sessao"
}

type chaveCtx struct{}

func usuarioID(r *http.Request) int64 { return r.Context().Value(chaveCtx{}).(int64) }

// pesado coloca a requisição numa fila com h.cfg.limitePDF vagas. Assim o pico
// de memória fica limitado (cada uma pode carregar um PDF de até 20 MiB
// várias vezes) e o excesso espera em vez de derrubar o servidor.
func (h *Handler) pesado(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		espera := time.NewTimer(h.cfg.esperaMaxima)
		defer espera.Stop()
		select {
		case h.vagas <- struct{}{}:
			defer func() { <-h.vagas }()
			next(w, r)
		case <-espera.C:
			w.Header().Set("Retry-After", "5")
			erroJSON(w, http.StatusServiceUnavailable, "servidor ocupado: muitas operações ao mesmo tempo, tente de novo em instantes")
		case <-r.Context().Done():
			// o cliente desistiu enquanto esperava
		}
	}
}

func (h *Handler) autenticado(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(h.nomeCookie())
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
