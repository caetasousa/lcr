package domain

import "time"

// MotivoRevogacao segue os códigos CRLReason da RFC 5280. O valor 0 é o
// próprio código "unspecified" da RFC; por isso a validade é checada com
// Valido() em vez de depender do zero value.
type MotivoRevogacao int

const (
	MotivoNaoEspecificado   MotivoRevogacao = 0
	MotivoChaveComprometida MotivoRevogacao = 1
	MotivoMudancaVinculo    MotivoRevogacao = 3
	MotivoSubstituido       MotivoRevogacao = 4
	MotivoCessacaoOperacao  MotivoRevogacao = 5
)

var nomesMotivo = map[MotivoRevogacao]string{
	MotivoNaoEspecificado:   "Não especificado",
	MotivoChaveComprometida: "Chave comprometida",
	MotivoMudancaVinculo:    "Mudança de vínculo",
	MotivoSubstituido:       "Substituído",
	MotivoCessacaoOperacao:  "Cessação de operação",
}

func (m MotivoRevogacao) Valido() bool { _, ok := nomesMotivo[m]; return ok }

func (m MotivoRevogacao) String() string {
	if n, ok := nomesMotivo[m]; ok {
		return n
	}
	return "Desconhecido"
}

// Situacao do certificado. O zero value é inválido de propósito.
type Situacao int

const (
	SituacaoDesconhecida Situacao = iota
	SituacaoValido
	SituacaoRevogado
	SituacaoExpirado
)

func (s Situacao) String() string {
	switch s {
	case SituacaoValido:
		return "válido"
	case SituacaoRevogado:
		return "revogado"
	case SituacaoExpirado:
		return "expirado"
	default:
		return "desconhecido"
	}
}

// Certificado é o certificado X.509 de um usuário, visto pelo domínio:
// interessa o ciclo de vida, não o formato ASN.1.
type Certificado struct {
	ID          int64
	UsuarioID   int64
	Serial      string // hexadecimal maiúsculo
	CertPEM     []byte
	ChaveSelada []byte // chave privada cifrada com a senha do titular
	EmitidoEm   time.Time
	ExpiraEm    time.Time
	RevogadoEm  *time.Time
	Motivo      *MotivoRevogacao
}

func (c *Certificado) Situacao(agora time.Time) Situacao {
	switch {
	case c.RevogadoEm != nil:
		return SituacaoRevogado
	case agora.After(c.ExpiraEm):
		return SituacaoExpirado
	default:
		return SituacaoValido
	}
}

// PodeAssinar: só um certificado válido assina.
func (c *Certificado) PodeAssinar(agora time.Time) error {
	switch c.Situacao(agora) {
	case SituacaoRevogado:
		return ErrCertificadoRevogado
	case SituacaoExpirado:
		return ErrCertificadoExpirado
	}
	return nil
}

// Revogar marca o certificado; a AC passa a listá-lo na LCR.
func (c *Certificado) Revogar(agora time.Time, motivo MotivoRevogacao) error {
	if !motivo.Valido() {
		return ErrMotivoInvalido
	}
	if c.RevogadoEm != nil {
		return ErrCertificadoJaRevogado
	}
	c.RevogadoEm, c.Motivo = &agora, &motivo
	return nil
}

// PodeSerSubstituido: só emitimos outro se o atual não serve mais.
func (c *Certificado) PodeSerSubstituido(agora time.Time) error {
	if c.Situacao(agora) == SituacaoValido {
		return ErrCertificadoAindaValido
	}
	return nil
}

// Revogacao é uma entrada da LCR.
type Revogacao struct {
	Serial  string
	Titular string
	Em      time.Time
	Motivo  MotivoRevogacao
}

// DetalhesCertificado é a leitura "humana" de um X.509, produzida pelo
// adaptador da AC.
type DetalhesCertificado struct {
	Titular      string
	Emissor      string
	Serial       string
	ValidoDe     time.Time
	ValidoAte    time.Time
	Algoritmo    string
	TamanhoChave int
	Fingerprint  string
	Usos         []string
	LCR          []string
	PEM          string
}

// InfoAC descreve a hierarquia e a LCR atual.
type InfoAC struct {
	Raiz          DetalhesCertificado
	Intermediaria DetalhesCertificado
	Revogados     []Revogacao
}

// Emissao é o resultado de a AC atender a uma solicitação: o certificado e
// a chave privada recém-gerada (PKCS#8 em claro, ainda não selada).
type Emissao struct {
	Serial     string
	CertPEM    []byte
	ChavePKCS8 []byte
	ValidoDe   time.Time
	ValidoAte  time.Time
}
