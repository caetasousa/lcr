package cripto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"lcr/internal/domain"
	"lcr/internal/port"
)

// Envelope implementa CifradorDocumento com cifra híbrida, o mesmo princípio
// do CMS EnvelopedData (S/MIME):
//
//	chave do documento (DEK) = 32 bytes aleatórios
//	conteudo = "d1" | nonce (12 B) | AES-256-GCM(DEK, nonce, PDF, AAD="lcr-documento-v1")
//	para cada signatário: RSA-OAEP-SHA256(chave pública do certificado, DEK, rótulo="lcr-documento")
//
// Só quem tem a chave privada de um dos certificados consegue recuperar a
// DEK, e o GCM garante que qualquer byte alterado no banco é detectado.
type Envelope struct{}

var _ port.CifradorDocumento = Envelope{}

const (
	versaoEnvelope = "d1"
	aadDocumento   = "lcr-documento-v1"
	rotuloOAEP     = "lcr-documento"
)

func (Envelope) Cifrar(pdf []byte, destinatarios [][]byte) ([]byte, [][]byte, error) {
	if len(destinatarios) == 0 {
		return nil, nil, errors.New("documento sem destinatários: ninguém conseguiria abri-lo")
	}
	dek, err := aleatorio(32)
	if err != nil {
		return nil, nil, err
	}
	defer zerar(dek)

	gcm, err := gcmDe(dek)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := aleatorio(gcm.NonceSize())
	if err != nil {
		return nil, nil, err
	}
	out := make([]byte, 0, len(versaoEnvelope)+len(nonce)+len(pdf)+gcm.Overhead())
	out = append(append(out, versaoEnvelope...), nonce...)
	conteudo := gcm.Seal(out, nonce, pdf, []byte(aadDocumento))

	chaves := make([][]byte, len(destinatarios))
	for i, certPEM := range destinatarios {
		pub, err := chavePublicaRSA(certPEM)
		if err != nil {
			return nil, nil, err
		}
		if chaves[i], err = rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, dek, []byte(rotuloOAEP)); err != nil {
			return nil, nil, fmt.Errorf("cifrando a chave do documento: %w", err)
		}
	}
	return conteudo, chaves, nil
}

func (Envelope) Decifrar(conteudo, chaveCifrada, chavePKCS8 []byte) ([]byte, error) {
	k, err := x509.ParsePKCS8PrivateKey(chavePKCS8)
	if err != nil {
		return nil, fmt.Errorf("chave privada inválida: %w", err)
	}
	priv, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("a chave privada não é RSA")
	}
	dek, err := rsa.DecryptOAEP(sha256.New(), nil, priv, chaveCifrada, []byte(rotuloOAEP))
	if err != nil {
		return nil, domain.ErrDocumentoCorrompido
	}
	defer zerar(dek)

	gcm, err := gcmDe(dek)
	if err != nil {
		return nil, err
	}
	cab := len(versaoEnvelope) + gcm.NonceSize()
	if len(conteudo) < cab+gcm.Overhead() || string(conteudo[:len(versaoEnvelope)]) != versaoEnvelope {
		return nil, domain.ErrDocumentoCorrompido
	}
	nonce := conteudo[len(versaoEnvelope):cab]
	pdf, err := gcm.Open(nil, nonce, conteudo[cab:], []byte(aadDocumento))
	if err != nil {
		// Tag do GCM não conferiu: o conteúdo foi alterado no banco.
		return nil, domain.ErrDocumentoCorrompido
	}
	return pdf, nil
}

func gcmDe(chave []byte) (cipher.AEAD, error) {
	bloco, err := aes.NewCipher(chave)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(bloco)
}

func chavePublicaRSA(certPEM []byte) (*rsa.PublicKey, error) {
	blk, _ := pem.Decode(certPEM)
	if blk == nil {
		return nil, errors.New("PEM de certificado inválido")
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := c.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("o certificado não tem chave pública RSA")
	}
	return pub, nil
}

func zerar(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
