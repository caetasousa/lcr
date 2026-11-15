// Package service implementa as portas primárias (casos de uso). Depende
// apenas do domínio e das portas secundárias: nenhum import de banco, HTTP
// ou biblioteca de PDF.
package service

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// Contextos (AAD do AES-GCM) usados ao selar chaves: um blob selado para
// um propósito não pode ser aberto em outro.
const contextoChaveUsuario = "chave-usuario"

type config struct {
	agora         func() time.Time
	duracaoSessao time.Duration
	urlPublica    string
	local         string
}

func configPadrao() config {
	return config{
		agora:         time.Now,
		duracaoSessao: 8 * time.Hour,
		urlPublica:    "http://localhost:8080",
		local:         "LCR Assina",
	}
}

// Opcao configura um serviço (functional options). Opções que validam
// devolvem erro na construção, e não em tempo de execução.
type Opcao func(*config) error

// ComRelogio injeta a fonte de tempo (útil em testes).
func ComRelogio(agora func() time.Time) Opcao {
	return func(c *config) error {
		if agora == nil {
			return errors.New("relógio nulo")
		}
		c.agora = agora
		return nil
	}
}

func ComDuracaoSessao(d time.Duration) Opcao {
	return func(c *config) error {
		if d <= 0 {
			return errors.New("duração de sessão deve ser positiva")
		}
		c.duracaoSessao = d
		return nil
	}
}

// ComURLPublica define a URL impressa no carimbo ("Verifique em ...").
func ComURLPublica(u string) Opcao {
	return func(c *config) error {
		p, err := url.Parse(u)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return errors.New("URL pública inválida: " + u)
		}
		c.urlPublica = strings.TrimRight(u, "/")
		return nil
	}
}

func ComLocal(local string) Opcao {
	return func(c *config) error {
		c.local = local
		return nil
	}
}

func aplica(opts []Opcao) (config, error) {
	c := configPadrao()
	for _, o := range opts {
		if err := o(&c); err != nil {
			return c, err
		}
	}
	return c, nil
}
