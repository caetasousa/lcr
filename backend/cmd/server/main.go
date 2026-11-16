// Comando server é a "raiz de composição" do hexágono: lê a configuração,
// constrói os adaptadores secundários, injeta-os nos serviços e entrega os
// serviços ao adaptador primário HTTP. Nenhuma regra de negócio mora aqui.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // fuso America/Sao_Paulo mesmo em imagens mínimas

	httpadapter "lcr/internal/adapter/primary/http"
	"lcr/internal/adapter/secondary/acx509"
	"lcr/internal/adapter/secondary/cripto"
	"lcr/internal/adapter/secondary/pades"
	"lcr/internal/adapter/secondary/postgres"
	"lcr/internal/service"
)

type configuracao struct {
	databaseURL  string
	senhaAC      string
	urlPublica   string
	porta        string
	estatico     string
	cookieSeguro bool
	fuso         *time.Location
	limiteArgon2 int // cálculos Argon2id simultâneos (64 MiB cada)
	limitePDF    int // requisições com PDF simultâneas
	cabecalhoIP  string
	semTaxa      bool
}

// Credenciais de desenvolvimento publicadas no .env.example. Servem para
// rodar localmente, mas são conhecidas de qualquer pessoa.
var segredosDeDesenvolvimento = []string{"lcr-local-dev-ac-2026", "lcr-local-dev-pg-2026", "troque-esta-senha-da-ac", "acme-dev-123"}

func inteiroPositivo(k string, padrao int) (int, error) {
	v := os.Getenv(k)
	if v == "" {
		return padrao, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s deve ser um inteiro >= 1, veio %q", k, v)
	}
	return n, nil
}

func env(k, padrao string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return padrao
}

// lerConfiguracao segue o 12-factor: tudo vem do ambiente e é validado
// na partida (fail fast).
func lerConfiguracao() (configuracao, error) {
	fuso, err := time.LoadLocation(env("TZ_ASSINATURA", "America/Sao_Paulo"))
	if err != nil {
		return configuracao{}, fmt.Errorf("TZ_ASSINATURA: %w", err)
	}
	c := configuracao{
		databaseURL:  env("DATABASE_URL", "postgres://acme:acme@localhost:5432/acme?sslmode=disable"),
		senhaAC:      os.Getenv("AC_SENHA"),
		urlPublica:   env("PUBLIC_URL", "http://localhost:8080"),
		porta:        env("PORT", "8080"),
		estatico:     os.Getenv("STATIC_DIR"),
		cookieSeguro: os.Getenv("COOKIE_SECURE") == "true",
		fuso:         fuso,
	}
	var errL error
	if c.limiteArgon2, errL = inteiroPositivo("LIMITE_ARGON2", 4); errL != nil {
		return c, errL
	}
	if c.limitePDF, errL = inteiroPositivo("LIMITE_PDF", 4); errL != nil {
		return c, errL
	}
	c.cabecalhoIP = os.Getenv("IP_HEADER")
	c.semTaxa = os.Getenv("RATE_LIMIT") == "off"
	if len(c.senhaAC) < 12 {
		return c, errors.New("defina AC_SENHA (mínimo 12 caracteres): ela protege as chaves privadas da AC")
	}
	// Em produção (HTTPS) o servidor se recusa a subir com segredos públicos
	// ou com as proteções desligadas: melhor falhar na partida do que rodar
	// inseguro sem ninguém perceber.
	if c.cookieSeguro {
		for _, s := range segredosDeDesenvolvimento {
			if c.senhaAC == s || strings.Contains(c.databaseURL, s) {
				return c, errors.New("produção (COOKIE_SECURE=true) com senha de desenvolvimento: gere AC_SENHA e POSTGRES_PASSWORD próprios")
			}
		}
		if c.semTaxa {
			return c, errors.New("RATE_LIMIT=off não é permitido em produção (COOKIE_SECURE=true)")
		}
		if !strings.HasPrefix(c.urlPublica, "https://") {
			return c, errors.New("produção (COOKIE_SECURE=true) exige PUBLIC_URL com https://")
		}
	}
	return c, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("falha fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := lerConfiguracao()
	if err != nil {
		return err
	}
	relogio := func() time.Time { return time.Now().In(cfg.fuso) }

	// ---------- adaptadores secundários ----------
	db, err := postgres.Conectar(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	parametros := cripto.ParametrosPadrao
	parametros.Simultaneos = cfg.limiteArgon2
	argon, err := cripto.NovoArgon2(parametros)
	if err != nil {
		return err
	}
	var (
		usuarios     = postgres.NovoUsuarios(db)
		certificados = postgres.NovoCertificados(db)
		sessoes      = postgres.NovoSessoes(db)
		documentos   = postgres.NovoDocumentos(db)
	)
	ac, err := acx509.Carregar(ctx, postgres.NovoArmazemAC(db), argon, cfg.senhaAC,
		acx509.ComURLPublica(cfg.urlPublica), acx509.ComRelogio(relogio))
	if err != nil {
		return err
	}
	slog.Info("AC pronta", "raiz", acx509.CNRaiz, "emissora", ac.NomeEmissor(), "sha256_raiz", ac.FingerprintRaiz())

	validador, err := pades.NovoValidador(ac.RaizPEM())
	if err != nil {
		return err
	}

	// ---------- serviços (portas primárias) ----------
	opts := []service.Opcao{service.ComRelogio(relogio), service.ComURLPublica(cfg.urlPublica)}
	conta, err := service.NovaConta(service.ContaDeps{
		Usuarios: usuarios, Certificados: certificados, Sessoes: sessoes,
		Hasher: argon, Cofre: argon, Tokens: cripto.Tokens{}, AC: ac,
	}, opts...)
	if err != nil {
		return err
	}
	certSvc, err := service.NovoCertificados(service.CertificadoDeps{
		Usuarios: usuarios, Certificados: certificados, Hasher: argon, Cofre: argon, AC: ac,
	}, opts...)
	if err != nil {
		return err
	}
	assinSvc, err := service.NovoAssinaturas(service.AssinaturaDeps{
		Usuarios: usuarios, Certificados: certificados, Documentos: documentos,
		Cofre: argon, AC: ac, Assinador: pades.Assinador{},
		Validador: validador, Cifrador: cripto.Envelope{},
	}, opts...)
	if err != nil {
		return err
	}
	// Documentos gravados em claro por versões antigas passam a ser cifrados.
	if n, err := assinSvc.MigrarLegados(ctx); err != nil {
		return fmt.Errorf("migrando documentos para armazenamento cifrado: %w", err)
	} else if n > 0 {
		slog.Info("documentos antigos cifrados", "quantidade", n)
	}

	// ---------- adaptador primário ----------
	opcoesHTTP := []httpadapter.Opcao{
		httpadapter.ComDiretorioEstatico(cfg.estatico), httpadapter.ComCookieSeguro(cfg.cookieSeguro),
		httpadapter.ComLimiteDePDFs(cfg.limitePDF, 30*time.Second),
	}
	if cfg.cabecalhoIP != "" {
		opcoesHTTP = append(opcoesHTTP, httpadapter.ComCabecalhoDeIP(cfg.cabecalhoIP))
	}
	if cfg.semTaxa {
		slog.Warn("limites de tentativas DESLIGADOS (RATE_LIMIT=off): use só em teste de carga")
		opcoesHTTP = append(opcoesHTTP, httpadapter.SemLimiteDeTaxa())
	}
	handler, err := httpadapter.Novo(httpadapter.Servicos{
		Conta:        conta,
		Certificados: certSvc,
		Assinaturas:  assinSvc,
		Validacao:    service.NovaValidacao(service.ValidacaoDeps{Certificados: certificados, AC: ac, Validador: validador}),
		AC:           service.NovaAC(certificados, ac),
	}, opcoesHTTP...)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + cfg.porta,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	erros := make(chan error, 1)
	go func() {
		slog.Info("servidor ouvindo", "addr", srv.Addr, "url", cfg.urlPublica)
		erros <- srv.ListenAndServe()
	}()

	select {
	case err := <-erros:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("encerrando: aguardando requisições em andamento")
	}
	desligar, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(desligar)
}
