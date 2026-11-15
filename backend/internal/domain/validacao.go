package domain

import "time"

// CertInfo é um elo da cadeia de certificação.
type CertInfo struct {
	Titular   string
	Emissor   string
	Serial    string
	ValidoDe  time.Time
	ValidoAte time.Time
}

// ConsultaLCR é a resposta da AC sobre um número de série.
type ConsultaLCR struct {
	NumeroLCR  string
	Revogado   bool
	RevogadoEm time.Time
	Motivo     MotivoRevogacao
}

// ResultadoAssinatura reúne as verificações técnicas (feitas pelo adaptador
// PAdES) e a decisão de negócio (feita aqui, em Avaliar).
type ResultadoAssinatura struct {
	Nome            string
	Motivo          string
	Local           string
	Contato         string
	DataAssinatura  *time.Time
	Integra         bool // hash + assinatura RSA conferem
	CadeiaConfiavel bool // chega até a raiz confiável
	RevogadoNoPDF   bool // revogação informada dentro do próprio PDF
	Certificado     *CertInfo
	Cadeia          []CertInfo
	Erros           []string
	Avisos          []string

	LCR              ConsultaLCR
	RevogadoAntes    bool // revogado antes do momento da assinatura
	EmitidoPorEstaAC bool
	Valida           bool
}

// Avaliar aplica a regra: uma assinatura é válida se o documento está
// íntegro, a cadeia é confiável, não há erros e o certificado NÃO estava
// revogado no momento da assinatura. Revogação posterior não invalida o
// que já foi assinado (a data aqui é a declarada; com carimbo de tempo de
// uma ACT ela seria garantida).
func (r *ResultadoAssinatura) Avaliar(lcr ConsultaLCR) {
	r.LCR = lcr
	r.RevogadoAntes = lcr.Revogado && (r.DataAssinatura == nil || lcr.RevogadoEm.Before(*r.DataAssinatura))
	r.Valida = r.Integra && r.CadeiaConfiavel && !r.RevogadoNoPDF && !r.RevogadoAntes && len(r.Erros) == 0
}

// ResultadoValidacao de um PDF inteiro.
type ResultadoValidacao struct {
	Paginas     int
	SHA256      string
	Assinaturas []ResultadoAssinatura
	Erro        string
	Valido      bool
}

// Consolidar: o documento é válido se tem pelo menos uma assinatura e
// todas são válidas.
func (v *ResultadoValidacao) Consolidar() {
	v.Valido = len(v.Assinaturas) > 0 && v.Erro == ""
	for _, a := range v.Assinaturas {
		v.Valido = v.Valido && a.Valida
	}
}
