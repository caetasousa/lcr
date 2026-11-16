package httpadapter

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Limitador de taxa (em memória, janela fixa por chave)
// ---------------------------------------------------------------------------

// limitador conta eventos por chave (IP, CPF, usuário) dentro de uma janela.
// É propositalmente simples: um mapa protegido por mutex, com limpeza
// periódica e teto de entradas para um atacante não esgotar a memória
// criando chaves.
type limitador struct {
	mu      sync.Mutex
	max     int
	janela  time.Duration
	agora   func() time.Time
	chaves  map[string]*contagem
	limpeza time.Time
}

type contagem struct {
	inicio time.Time
	n      int
}

const maxChavesLimitador = 50_000

func novoLimitador(max int, janela time.Duration) *limitador {
	return &limitador{max: max, janela: janela, agora: time.Now, chaves: map[string]*contagem{}}
}

// atual devolve a contagem viva da chave (criando ou reiniciando a janela).
// Chamar com o mutex travado.
func (l *limitador) atual(chave string) *contagem {
	agora := l.agora()
	if agora.Sub(l.limpeza) > l.janela || len(l.chaves) > maxChavesLimitador {
		for k, c := range l.chaves {
			if agora.Sub(c.inicio) >= l.janela {
				delete(l.chaves, k)
			}
		}
		if len(l.chaves) > maxChavesLimitador { // sob ataque: recomeça do zero
			l.chaves = map[string]*contagem{}
		}
		l.limpeza = agora
	}
	c, ok := l.chaves[chave]
	if !ok || agora.Sub(c.inicio) >= l.janela {
		c = &contagem{inicio: agora}
		l.chaves[chave] = c
	}
	return c
}

// permite conta um evento e diz se ele cabe no limite. Quando não cabe,
// devolve em quanto tempo a janela termina.
func (l *limitador) permite(chave string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.atual(chave)
	if c.n >= l.max {
		return false, l.janela - l.agora().Sub(c.inicio)
	}
	c.n++
	return true, 0
}

// bloqueado diz se a chave já estourou o limite, sem contar um novo evento.
// Usado para contar só as FALHAS (senha errada) e não as tentativas legítimas.
func (l *limitador) bloqueado(chave string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.atual(chave)
	if c.n >= l.max {
		return true, l.janela - l.agora().Sub(c.inicio)
	}
	return false, 0
}

func (l *limitador) falhou(chave string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.atual(chave).n++
}

func (l *limitador) zera(chave string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.chaves, chave)
}

// limites agrupa os limitadores do adaptador. nil = limitação desligada.
type limites struct {
	geral        *limitador // qualquer rota /api/, por IP
	login        *limitador // tentativas de login, por IP
	loginFalhas  *limitador // senhas erradas, por CPF
	cadastro     *limitador // contas novas, por IP (cada uma gera um par RSA)
	validar      *limitador // validações públicas, por IP
	senhaFalhas  *limitador // senha errada em operação autenticada, por usuário
	cadastroErro *limitador // cadastros recusados, por IP (sondagem de CPFs)
}

func limitesPadrao() *limites {
	return &limites{
		geral:        novoLimitador(600, time.Minute),
		login:        novoLimitador(30, time.Minute),
		loginFalhas:  novoLimitador(8, 15*time.Minute),
		cadastro:     novoLimitador(30, 10*time.Minute),
		validar:      novoLimitador(60, time.Minute),
		senhaFalhas:  novoLimitador(8, 15*time.Minute),
		cadastroErro: novoLimitador(20, 10*time.Minute),
	}
}

// muitasRequisicoes responde 429 com Retry-After.
func muitasRequisicoes(w http.ResponseWriter, espera time.Duration, msg string) {
	seg := int(espera.Seconds()) + 1
	w.Header().Set("Retry-After", strconv.Itoa(seg))
	erroJSON(w, http.StatusTooManyRequests, fmt.Sprintf("%s Tente de novo em %s.", msg, duracaoHumana(espera)))
}

func duracaoHumana(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d s", int(d.Seconds())+1)
	}
	return fmt.Sprintf("%d min", int(d.Minutes())+1)
}

// ---------------------------------------------------------------------------
// IP do cliente e auditoria
// ---------------------------------------------------------------------------

// ipCliente devolve o IP de quem chamou. Atrás de um proxy confiável (em
// produção, o túnel da Cloudflare) o IP real vem num cabeçalho que o proxy
// sobrescreve; sem proxy, esse cabeçalho é ignorado porque o cliente poderia
// forjá-lo.
func (h *Handler) ipCliente(r *http.Request) string {
	if h.cfg.cabecalhoIP != "" {
		if v := strings.TrimSpace(r.Header.Get(h.cfg.cabecalhoIP)); v != "" {
			if ip := net.ParseIP(v); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// auditar registra eventos de segurança em formato estruturado. Nunca
// recebe senha, token nem CPF completo.
func (h *Handler) auditar(r *http.Request, evento string, attrs ...any) {
	base := []any{"evento", evento, "ip", h.ipCliente(r), "rota", r.Method + " " + r.URL.Path}
	if id, ok := r.Context().Value(chaveCtx{}).(int64); ok {
		base = append(base, "usuario", id)
	}
	slog.InfoContext(r.Context(), "auditoria", append(base, attrs...)...)
}

// cpfParaLog mostra só o miolo do CPF, como no carimbo (***.982.247-**).
func cpfParaLog(cpf string) string {
	var d []rune
	for _, c := range cpf {
		if c >= '0' && c <= '9' {
			d = append(d, c)
		}
	}
	if len(d) != 11 {
		return "inválido"
	}
	return "***." + string(d[3:6]) + "." + string(d[6:9]) + "-**"
}

// ---------------------------------------------------------------------------
// Middlewares globais
// ---------------------------------------------------------------------------

// recuperar transforma um panic (por exemplo, de um PDF malformado dentro de
// uma biblioteca) em 500, sem derrubar a conexão nem vazar detalhes.
func (h *Handler) recuperar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.ErrorContext(r.Context(), "panic na requisição", "rota", r.URL.Path, "erro", rec, "pilha", string(debug.Stack()))
				erroJSON(w, http.StatusInternalServerError, "erro interno no servidor")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// politicaConteudo é a Content-Security-Policy da aplicação inteira: só
// carrega recursos da própria origem, nunca é embutida em outro site e não
// executa scripts inline. 'unsafe-inline' fica restrito a ESTILOS (o React
// usa o atributo style); blob: é usado pelo worker do pdf.js.
const politicaConteudo = "default-src 'self'; " +
	"script-src 'self' 'wasm-unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"worker-src 'self' blob:; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

func (h *Handler) cabecalhosSeguranca(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := w.Header()
		c.Set("Content-Security-Policy", politicaConteudo)
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		c.Set("Cross-Origin-Opener-Policy", "same-origin")
		if h.cfg.cookieSeguro {
			// Só faz sentido (e só é respeitado) em HTTPS.
			c.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// Respostas da API levam dados pessoais, PDFs e certificados:
			// nenhum cache intermediário ou do navegador deve guardá-las.
			c.Set("Cache-Control", "no-store")
			// Os certificados e a LCR da AC são públicos por natureza e
			// precisam ser buscáveis por qualquer validador.
			if !strings.HasPrefix(r.URL.Path, "/api/ac/") {
				c.Set("Cross-Origin-Resource-Policy", "same-origin")
			}
		}
		next.ServeHTTP(w, r)
	})
}

// mesmaOrigem barra requisições de escrita vindas de OUTRO site (CSRF). O
// cookie SameSite=Strict não basta aqui: subdomínios irmãos (outro app em
// *.d4ffy.com) contam como "mesmo site" para o navegador. Por isso conferimos
// a origem exata: Sec-Fetch-Site (navegadores atuais) ou Origin.
// Clientes que não são navegador (curl, scripts) não enviam esses cabeçalhos
// e não são vetor de CSRF, então passam.
func (h *Handler) mesmaOrigem(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		ok := true
		if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
			ok = sfs == "same-origin" || sfs == "none"
		} else if origem := r.Header.Get("Origin"); origem != "" {
			u, err := url.Parse(origem)
			ok = err == nil && strings.EqualFold(u.Host, r.Host)
		}
		if !ok {
			h.auditar(r, "origem_rejeitada", "origin", r.Header.Get("Origin"), "sec_fetch_site", r.Header.Get("Sec-Fetch-Site"))
			erroJSON(w, http.StatusForbidden, "requisição de outra origem recusada")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// taxaGeral limita o total de chamadas à API por IP.
func (h *Handler) taxaGeral(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.lim != nil && strings.HasPrefix(r.URL.Path, "/api/") {
			if ok, espera := h.lim.geral.permite(h.ipCliente(r)); !ok {
				h.auditar(r, "taxa_excedida", "limite", "geral")
				muitasRequisicoes(w, espera, "Muitas requisições.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Middlewares por rota
// ---------------------------------------------------------------------------

// porIP aplica um limitador por IP a uma rota pública.
func (h *Handler) porIP(escolhe func(*limites) *limitador, nome string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.lim != nil {
			if ok, espera := escolhe(h.lim).permite(h.ipCliente(r)); !ok {
				h.auditar(r, "taxa_excedida", "limite", nome)
				muitasRequisicoes(w, espera, "Muitas tentativas a partir deste endereço.")
				return
			}
		}
		next(w, r)
	}
}

// capturaStatus guarda o status que o handler escreveu.
type capturaStatus struct {
	http.ResponseWriter
	status int
}

func (c *capturaStatus) WriteHeader(s int) {
	if c.status == 0 {
		c.status = s
	}
	c.ResponseWriter.WriteHeader(s)
}

func (c *capturaStatus) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.ResponseWriter.Write(b)
}

// protegeSenha envolve as rotas autenticadas que pedem a senha de novo
// (assinar, baixar, exportar, revogar, renovar). Quem roubar uma sessão não
// pode usá-las para adivinhar a senha: depois de N senhas erradas o usuário
// fica bloqueado por um tempo nessas operações. Também registra cada
// operação sensível na trilha de auditoria.
func (h *Handler) protegeSenha(evento string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chave := strconv.FormatInt(usuarioID(r), 10)
		if h.lim != nil {
			if bloq, espera := h.lim.senhaFalhas.bloqueado(chave); bloq {
				h.auditar(r, "taxa_excedida", "limite", "senha")
				muitasRequisicoes(w, espera, "Muitas senhas incorretas.")
				return
			}
		}
		cs := &capturaStatus{ResponseWriter: w}
		next(cs, r)
		switch {
		case cs.status == http.StatusForbidden: // senha incorreta
			if h.lim != nil {
				h.lim.senhaFalhas.falhou(chave)
			}
			h.auditar(r, evento, "resultado", "senha_incorreta")
		case cs.status >= 200 && cs.status < 300:
			if h.lim != nil {
				h.lim.senhaFalhas.zera(chave)
			}
			h.auditar(r, evento, "resultado", "ok")
		default:
			h.auditar(r, evento, "resultado", "recusado", "status", cs.status)
		}
	}
}
