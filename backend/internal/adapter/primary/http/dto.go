package httpadapter

import (
	"time"

	"lcr/internal/domain"
)

// DTOs: o formato JSON é responsabilidade do adaptador; o domínio não tem
// tags de serialização.

type certificadoDTO struct {
	Titular         string     `json:"titular"`
	Emissor         string     `json:"emissor"`
	Serial          string     `json:"serial"`
	ValidoDe        time.Time  `json:"valido_de"`
	ValidoAte       time.Time  `json:"valido_ate"`
	Algoritmo       string     `json:"algoritmo"`
	TamanhoChave    int        `json:"tamanho_chave"`
	Fingerprint     string     `json:"fingerprint_sha256"`
	Usos            []string   `json:"usos_chave"`
	LCR             []string   `json:"lcr"`
	RevogadoEm      *time.Time `json:"revogado_em,omitempty"`
	MotivoRevogacao string     `json:"motivo_revogacao,omitempty"`
	Situacao        string     `json:"situacao"`
	PEM             string     `json:"pem"`
}

func paraCertificadoDTO(c *domain.Certificado, d *domain.DetalhesCertificado, agora time.Time) certificadoDTO {
	dto := detalhesDTO(d)
	dto.Situacao = c.Situacao(agora).String()
	dto.RevogadoEm = c.RevogadoEm
	if c.Motivo != nil {
		dto.MotivoRevogacao = c.Motivo.String()
	}
	return dto
}

func detalhesDTO(d *domain.DetalhesCertificado) certificadoDTO {
	usos, lcr := d.Usos, d.LCR
	if usos == nil {
		usos = []string{}
	}
	if lcr == nil {
		lcr = []string{}
	}
	return certificadoDTO{
		Titular: d.Titular, Emissor: d.Emissor, Serial: d.Serial, ValidoDe: d.ValidoDe, ValidoAte: d.ValidoAte,
		Algoritmo: d.Algoritmo, TamanhoChave: d.TamanhoChave, Fingerprint: d.Fingerprint,
		Usos: usos, LCR: lcr, PEM: d.PEM, Situacao: domain.SituacaoValido.String(),
	}
}

type certInfoDTO struct {
	Titular   string    `json:"titular"`
	Emissor   string    `json:"emissor"`
	Serial    string    `json:"serial"`
	ValidoDe  time.Time `json:"valido_de"`
	ValidoAte time.Time `json:"valido_ate"`
}

type lcrDTO struct {
	EmitidoPorEstaAC  bool       `json:"emitido_por_esta_ac"`
	NumeroLCR         string     `json:"numero_lcr"`
	RevogadoEm        *time.Time `json:"revogado_em"`
	Motivo            string     `json:"motivo,omitempty"`
	AntesDaAssinatura bool       `json:"revogado_antes_da_assinatura"`
}

type assinaturaDTO struct {
	Nome            string        `json:"nome"`
	Motivo          string        `json:"motivo,omitempty"`
	Local           string        `json:"local,omitempty"`
	Contato         string        `json:"contato,omitempty"`
	DataAssinatura  *time.Time    `json:"data_assinatura,omitempty"`
	Integra         bool          `json:"integra"`
	CadeiaConfiavel bool          `json:"cadeia_confiavel"`
	RevogadoNoPDF   bool          `json:"revogado_na_lcr"`
	Certificado     *certInfoDTO  `json:"certificado,omitempty"`
	Cadeia          []certInfoDTO `json:"cadeia"`
	Erros           []string      `json:"erros"`
	Avisos          []string      `json:"avisos"`
	LCR             lcrDTO        `json:"lcr"`
	Valida          bool          `json:"valida"`
}

type validacaoDTO struct {
	Paginas     int             `json:"paginas"`
	SHA256      string          `json:"sha256"`
	Assinaturas []assinaturaDTO `json:"assinaturas"`
	Erro        string          `json:"erro,omitempty"`
	Valido      bool            `json:"valido"`
}

func paraCertInfo(c domain.CertInfo) certInfoDTO {
	return certInfoDTO{Titular: c.Titular, Emissor: c.Emissor, Serial: c.Serial, ValidoDe: c.ValidoDe, ValidoAte: c.ValidoAte}
}

func paraValidacaoDTO(v *domain.ResultadoValidacao) validacaoDTO {
	out := validacaoDTO{Paginas: v.Paginas, SHA256: v.SHA256, Erro: v.Erro, Valido: v.Valido, Assinaturas: []assinaturaDTO{}}
	for _, a := range v.Assinaturas {
		d := assinaturaDTO{
			Nome: a.Nome, Motivo: a.Motivo, Local: a.Local, Contato: a.Contato, DataAssinatura: a.DataAssinatura,
			Integra: a.Integra, CadeiaConfiavel: a.CadeiaConfiavel, RevogadoNoPDF: a.RevogadoNoPDF,
			Cadeia: []certInfoDTO{}, Erros: a.Erros, Avisos: a.Avisos, Valida: a.Valida,
			LCR: lcrDTO{
				EmitidoPorEstaAC: a.EmitidoPorEstaAC, NumeroLCR: a.LCR.NumeroLCR,
				AntesDaAssinatura: a.RevogadoAntes,
			},
		}
		if a.LCR.Revogado {
			t := a.LCR.RevogadoEm
			d.LCR.RevogadoEm, d.LCR.Motivo = &t, a.LCR.Motivo.String()
		}
		if a.Certificado != nil {
			c := paraCertInfo(*a.Certificado)
			d.Certificado = &c
		}
		for _, c := range a.Cadeia {
			d.Cadeia = append(d.Cadeia, paraCertInfo(c))
		}
		out.Assinaturas = append(out.Assinaturas, d)
	}
	return out
}

type documentoDTO struct {
	ID             int64     `json:"id"`
	Nome           string    `json:"nome"`
	Pagina         int       `json:"pagina"`
	AssinadoEm     time.Time `json:"assinado_em"`
	SHA256Original string    `json:"sha256_original"`
	SHA256Assinado string    `json:"sha256_assinado"`
	Tamanho        int       `json:"tamanho"`
}

type revogadoDTO struct {
	Serial     string    `json:"serial"`
	Titular    string    `json:"titular"`
	RevogadoEm time.Time `json:"revogado_em"`
	Motivo     string    `json:"motivo"`
}

type acDTO struct {
	Raiz          certificadoDTO `json:"raiz"`
	Intermediaria certificadoDTO `json:"intermediaria"`
	Revogados     []revogadoDTO  `json:"revogados"`
}
