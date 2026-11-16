package httpadapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLimitadorBloqueiaEReabreDepoisDaJanela(t *testing.T) {
	agora := time.Unix(1_000_000, 0)
	l := novoLimitador(3, time.Minute)
	l.agora = func() time.Time { return agora }

	for i := range 3 {
		if ok, _ := l.permite("ip"); !ok {
			t.Fatalf("tentativa %d deveria passar", i+1)
		}
	}
	if ok, espera := l.permite("ip"); ok || espera <= 0 || espera > time.Minute {
		t.Errorf("a 4ª deveria ser barrada com espera válida: ok=%v espera=%v", ok, espera)
	}
	if ok, _ := l.permite("outro-ip"); !ok {
		t.Error("o limite é por chave: outro IP não pode ser afetado")
	}
	agora = agora.Add(61 * time.Second)
	if ok, _ := l.permite("ip"); !ok {
		t.Error("depois da janela o acesso deve voltar")
	}
}

func TestLimitadorContaSoFalhasEZeraNoSucesso(t *testing.T) {
	l := novoLimitador(2, time.Minute)
	if b, _ := l.bloqueado("cpf"); b {
		t.Fatal("consultar não pode contar")
	}
	l.falhou("cpf")
	l.falhou("cpf")
	if b, _ := l.bloqueado("cpf"); !b {
		t.Error("duas falhas deveriam bloquear")
	}
	l.zera("cpf")
	if b, _ := l.bloqueado("cpf"); b {
		t.Error("o sucesso zera a contagem")
	}
}

func TestLimitadorNaoCresceSemLimite(t *testing.T) {
	l := novoLimitador(1, time.Hour)
	for i := range maxChavesLimitador + 500 {
		l.permite(strings.Repeat("x", i%7) + string(rune(i)))
	}
	if n := len(l.chaves); n > maxChavesLimitador+1 {
		t.Errorf("o mapa deveria ter teto, tem %d chaves", n)
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestMesmaOrigemBarraOutrosSitesESubdominiosIrmaos(t *testing.T) {
	h := &Handler{}
	casos := []struct {
		nome, metodo, sfs, origin string
		quer                      int
	}{
		{"mesma origem (Sec-Fetch-Site)", "POST", "same-origin", "", 200},
		{"digitado pelo usuário", "POST", "none", "", 200},
		{"outro site", "POST", "cross-site", "https://malicioso.example", 403},
		{"subdomínio irmão (same-site)", "POST", "same-site", "https://studygo.d4ffy.com", 403},
		{"navegador antigo, Origin igual ao Host", "POST", "", "https://lcr.d4ffy.com", 200},
		{"navegador antigo, Origin de outro host", "POST", "", "https://outro.d4ffy.com", 403},
		{"curl/script (sem cabeçalhos)", "POST", "", "", 200},
		{"leitura nunca é barrada", "GET", "cross-site", "https://malicioso.example", 200},
	}
	for _, c := range casos {
		r := httptest.NewRequest(c.metodo, "https://lcr.d4ffy.com/api/login", nil)
		if c.sfs != "" {
			r.Header.Set("Sec-Fetch-Site", c.sfs)
		}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		w := httptest.NewRecorder()
		h.mesmaOrigem(okHandler()).ServeHTTP(w, r)
		if w.Code != c.quer {
			t.Errorf("%s: esperava %d, veio %d", c.nome, c.quer, w.Code)
		}
	}
}

func TestCabecalhosDeSeguranca(t *testing.T) {
	h := &Handler{cfg: config{cookieSeguro: true}}
	w := httptest.NewRecorder()
	h.cabecalhosSeguranca(okHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/api/eu", nil))
	for nome, contem := range map[string]string{
		"Content-Security-Policy":      "frame-ancestors 'none'",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Strict-Transport-Security":    "max-age=",
		"Cache-Control":                "no-store",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if v := w.Header().Get(nome); !strings.Contains(v, contem) {
			t.Errorf("%s = %q, deveria conter %q", nome, v, contem)
		}
	}
	if csp := w.Header().Get("Content-Security-Policy"); strings.Contains(strings.Split(csp, "style-src")[0], "unsafe-inline") {
		t.Error("script-src não pode permitir inline")
	}

	// Sem HTTPS não há HSTS; e os arquivos públicos da AC não levam CORP.
	h = &Handler{}
	w = httptest.NewRecorder()
	h.cabecalhosSeguranca(okHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/api/ac/lcr.crl", nil))
	if w.Header().Get("Strict-Transport-Security") != "" || w.Header().Get("Cross-Origin-Resource-Policy") != "" {
		t.Error("HSTS só em HTTPS e CORP fora de /api/ac/")
	}
}

func TestIPDoClienteSoConfiaNoCabecalhoQuandoConfigurado(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.9:5555"
	r.Header.Set("CF-Connecting-IP", "203.0.113.7")
	if ip := (&Handler{}).ipCliente(r); ip != "10.0.0.9" {
		t.Errorf("sem proxy configurado o cabeçalho é ignorado (forjável): %s", ip)
	}
	h := &Handler{cfg: config{cabecalhoIP: "Cf-Connecting-Ip"}}
	if ip := h.ipCliente(r); ip != "203.0.113.7" {
		t.Errorf("com proxy configurado vale o cabeçalho: %s", ip)
	}
	r.Header.Set("CF-Connecting-IP", "não é um ip")
	if ip := h.ipCliente(r); ip != "10.0.0.9" {
		t.Errorf("valor inválido cai para a conexão: %s", ip)
	}
}

func TestProtegeSenhaBloqueiaDepoisDeErrosSeguidos(t *testing.T) {
	h := &Handler{lim: limitesPadrao()}
	status := http.StatusForbidden // "senha incorreta"
	rota := h.protegeSenha("teste", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	chama := func() int {
		r := httptest.NewRequest("POST", "/api/assinar", nil)
		r = r.WithContext(context.WithValue(r.Context(), chaveCtx{}, int64(7)))
		w := httptest.NewRecorder()
		rota(w, r)
		return w.Code
	}
	for i := range 8 {
		if c := chama(); c != http.StatusForbidden {
			t.Fatalf("erro %d deveria ser 403, veio %d", i+1, c)
		}
	}
	if c := chama(); c != http.StatusTooManyRequests {
		t.Errorf("depois de 8 senhas erradas esperava 429, veio %d", c)
	}

	// Um acerto zera a contagem.
	h = &Handler{lim: limitesPadrao()}
	rota = h.protegeSenha("teste", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	for range 7 {
		chama()
	}
	status = http.StatusOK
	chama()
	status = http.StatusForbidden
	for range 7 {
		if c := chama(); c != http.StatusForbidden {
			t.Fatalf("contagem deveria ter sido zerada: %d", c)
		}
	}
}

func TestPanicViraErro500(t *testing.T) {
	h := &Handler{}
	w := httptest.NewRecorder()
	h.recuperar(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("pdf malformado") })).
		ServeHTTP(w, httptest.NewRequest("POST", "/api/validar", nil))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "malformado") {
		t.Errorf("esperava 500 sem detalhes: %d %s", w.Code, w.Body.String())
	}
}

func TestCookieUsaPrefixoHostEmHTTPS(t *testing.T) {
	if n := (&Handler{cfg: config{cookieSeguro: true}}).nomeCookie(); n != "__Host-sessao" {
		t.Errorf("em HTTPS o cookie deve ter prefixo __Host-: %s", n)
	}
	if n := (&Handler{}).nomeCookie(); n != "sessao" {
		t.Errorf("em HTTP (dev) o prefixo não funciona: %s", n)
	}
}

func TestCPFNuncaVaiInteiroParaOLog(t *testing.T) {
	if v := cpfParaLog("529.982.247-25"); v != "***.982.247-**" {
		t.Errorf("máscara errada: %s", v)
	}
	if v := cpfParaLog("123"); v != "inválido" {
		t.Errorf("CPF malformado: %s", v)
	}
}
