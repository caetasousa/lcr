package cripto_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"lcr/internal/adapter/secondary/cripto"
	"lcr/internal/domain"
)

// parDeChaves gera um certificado autoassinado (PEM) e a chave PKCS#8.
func parDeChaves(t *testing.T, cn string) (certPEM, chavePKCS8 []byte) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	p8, _ := x509.MarshalPKCS8PrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), p8
}

func TestEnvelopePreservaOArquivoByteAByte(t *testing.T) {
	certA, chaveA := parDeChaves(t, "A")
	certB, chaveB := parDeChaves(t, "B")
	_, chaveIntrusa := parDeChaves(t, "C")

	pdf := make([]byte, 300_000)
	_, _ = rand.Read(pdf)
	copy(pdf, "%PDF-1.7\n")

	conteudo, chaves, err := cripto.Envelope{}.Cifrar(pdf, [][]byte{certA, certB})
	if err != nil {
		t.Fatal(err)
	}
	if len(chaves) != 2 {
		t.Fatalf("esperava uma chave por signatário, veio %d", len(chaves))
	}
	if bytes.Contains(conteudo, []byte("%PDF")) {
		t.Error("o conteúdo cifrado não pode expor o PDF")
	}

	// Cada signatário abre com a própria chave e recebe exatamente os mesmos bytes.
	for i, c := range []struct{ chave, priv []byte }{{chaves[0], chaveA}, {chaves[1], chaveB}} {
		got, err := cripto.Envelope{}.Decifrar(conteudo, c.chave, c.priv)
		if err != nil {
			t.Fatalf("signatário %d: %v", i, err)
		}
		if !bytes.Equal(got, pdf) {
			t.Fatalf("signatário %d: o arquivo decifrado difere do original", i)
		}
	}

	// Quem não é signatário não abre.
	if _, err := (cripto.Envelope{}).Decifrar(conteudo, chaves[0], chaveIntrusa); !errors.Is(err, domain.ErrDocumentoCorrompido) {
		t.Errorf("chave de terceiro deveria falhar: %v", err)
	}

	// Um único bit alterado no banco é detectado pelo GCM.
	adulterado := bytes.Clone(conteudo)
	adulterado[len(adulterado)/2] ^= 1
	if _, err := (cripto.Envelope{}).Decifrar(adulterado, chaves[0], chaveA); !errors.Is(err, domain.ErrDocumentoCorrompido) {
		t.Errorf("conteúdo adulterado deveria falhar: %v", err)
	}
}

func TestEnvelopeExigeDestinatario(t *testing.T) {
	if _, _, err := (cripto.Envelope{}).Cifrar([]byte("%PDF-"), nil); err == nil {
		t.Error("cifrar sem destinatário deixaria o documento inacessível")
	}
}
