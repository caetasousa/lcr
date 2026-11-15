// Package domain contém as regras de negócio puras da assinatura digital:
// identidade do titular, ciclo de vida do certificado, posição do carimbo e
// o critério que decide se uma assinatura é válida. Não depende de banco,
// HTTP nem de bibliotecas de criptografia.
package domain

import "errors"

// Erros de negócio. Os adaptadores traduzem cada um para o seu protocolo
// (por exemplo, status HTTP); o domínio não sabe que HTTP existe.
var (
	ErrNaoEncontrado          = errors.New("registro não encontrado")
	ErrNomeInvalido           = errors.New("informe o nome completo (3 a 80 caracteres)")
	ErrCPFInvalido            = errors.New("CPF inválido")
	ErrSenhaFraca             = errors.New("a senha deve ter pelo menos 8 caracteres")
	ErrCPFJaCadastrado        = errors.New("já existe uma conta com este CPF")
	ErrCredenciaisInvalidas   = errors.New("CPF ou senha incorretos")
	ErrSessaoInvalida         = errors.New("sessão expirada, faça login novamente")
	ErrSenhaIncorreta         = errors.New("senha incorreta: a chave privada não pôde ser liberada")
	ErrCertificadoRevogado    = errors.New("seu certificado está revogado; emita um novo em Meu certificado")
	ErrCertificadoExpirado    = errors.New("seu certificado expirou; emita um novo em Meu certificado")
	ErrCertificadoJaRevogado  = errors.New("o certificado já está revogado")
	ErrCertificadoAindaValido = errors.New("o certificado atual ainda é válido; revogue-o antes de emitir outro")
	ErrMotivoInvalido         = errors.New("motivo de revogação inválido")
	ErrPosicaoInvalida        = errors.New("posição da assinatura inválida")
	ErrPDFInvalido            = errors.New("o arquivo não é um PDF válido")
)
