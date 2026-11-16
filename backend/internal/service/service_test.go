package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"lcr/internal/domain"
	"lcr/internal/port"
)

// ---------------------------------------------------------------------------
// Fakes em memória das portas secundárias. É aqui que a arquitetura
// hexagonal paga a conta: os casos de uso são testados sem banco, sem RSA
// e sem PDF real.
// ---------------------------------------------------------------------------

type memUsuarios struct {
	por map[domain.CPF]*domain.Usuario
}

func (m *memUsuarios) Criar(_ context.Context, u *domain.Usuario) error {
	if _, ok := m.por[u.CPF]; ok {
		return domain.ErrCPFJaCadastrado
	}
	u.ID = int64(len(m.por) + 1)
	m.por[u.CPF] = u
	return nil
}
func (m *memUsuarios) PorCPF(_ context.Context, c domain.CPF) (*domain.Usuario, error) {
	if u, ok := m.por[c]; ok {
		return u, nil
	}
	return nil, domain.ErrNaoEncontrado
}
func (m *memUsuarios) PorID(_ context.Context, id int64) (*domain.Usuario, error) {
	for _, u := range m.por {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrNaoEncontrado
}

type memCerts struct{ lista []*domain.Certificado }

func (m *memCerts) Criar(_ context.Context, c *domain.Certificado) error {
	c.ID = int64(len(m.lista) + 1)
	m.lista = append(m.lista, c)
	return nil
}
func (m *memCerts) Atual(_ context.Context, uid int64) (*domain.Certificado, error) {
	for i := len(m.lista) - 1; i >= 0; i-- {
		if m.lista[i].UsuarioID == uid {
			return m.lista[i], nil
		}
	}
	return nil, domain.ErrNaoEncontrado
}
func (m *memCerts) SalvarRevogacao(context.Context, *domain.Certificado) error { return nil }
func (m *memCerts) PorID(_ context.Context, id int64) (*domain.Certificado, error) {
	for _, c := range m.lista {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, domain.ErrNaoEncontrado
}
func (m *memCerts) PorSerial(_ context.Context, serial string) (*domain.Certificado, error) {
	for _, c := range m.lista {
		if c.Serial == serial {
			return c, nil
		}
	}
	return nil, domain.ErrNaoEncontrado
}
func (m *memCerts) Revogados(context.Context) ([]domain.Revogacao, error) {
	var out []domain.Revogacao
	for _, c := range m.lista {
		if c.RevogadoEm != nil {
			out = append(out, domain.Revogacao{Serial: c.Serial, Em: *c.RevogadoEm, Motivo: *c.Motivo})
		}
	}
	return out, nil
}

type memSessoes struct{ s map[string]int64 }

func (m *memSessoes) Criar(_ context.Context, h []byte, uid int64, _ time.Time) error {
	m.s[string(h)] = uid
	return nil
}
func (m *memSessoes) UsuarioDe(_ context.Context, h []byte, _ time.Time) (int64, error) {
	if id, ok := m.s[string(h)]; ok {
		return id, nil
	}
	return 0, domain.ErrNaoEncontrado
}
func (m *memSessoes) Apagar(_ context.Context, h []byte) error { delete(m.s, string(h)); return nil }

// memDocs guarda documentos e chaves de acesso como o banco faria.
type memDocs struct {
	certs  *memCerts
	docs   []domain.Documento
	chaves map[int64][]domain.ChaveDocumento
}

func (m *memDocs) Salvar(_ context.Context, d *domain.Documento, ch []domain.ChaveDocumento) error {
	d.ID = int64(len(m.docs) + 1)
	m.docs = append(m.docs, *d)
	m.chaves[d.ID] = ch
	return nil
}
func (m *memDocs) temAcesso(docID, uid int64) *domain.ChaveDocumento {
	for _, k := range m.chaves[docID] {
		if c, _ := m.certs.PorID(context.Background(), k.CertificadoID); c != nil && c.UsuarioID == uid {
			return &k
		}
	}
	return nil
}
func (m *memDocs) DoUsuario(_ context.Context, uid int64) ([]domain.Documento, error) {
	var out []domain.Documento
	for _, d := range m.docs {
		if m.temAcesso(d.ID, uid) != nil {
			out = append(out, d)
		}
	}
	return out, nil
}
func (m *memDocs) Buscar(_ context.Context, uid, id int64) (*domain.Documento, *domain.ChaveDocumento, error) {
	for _, d := range m.docs {
		if k := m.temAcesso(d.ID, uid); d.ID == id && k != nil {
			cp := d
			return &cp, k, nil
		}
	}
	return nil, nil, domain.ErrNaoEncontrado
}
func (m *memDocs) Legados(context.Context) ([]domain.Documento, error) { return nil, nil }
func (m *memDocs) CifrarLegado(context.Context, *domain.Documento, []domain.ChaveDocumento) error {
	return nil
}

// fakeCifrador: "cifra" invertendo os bytes; a chave de cada destinatário é
// o próprio PEM, e só abre com a chave privada "CHAVE".
type fakeCifrador struct{}

func inverte(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}
func (fakeCifrador) Cifrar(pdf []byte, dest [][]byte) ([]byte, [][]byte, error) {
	return inverte(pdf), dest, nil
}
func (fakeCifrador) Decifrar(c, _, priv []byte) ([]byte, error) {
	if string(priv) != "CHAVE" {
		return nil, domain.ErrDocumentoCorrompido
	}
	return inverte(c), nil
}

// fakeValidadorSignatarios devolve, como "assinaturas no PDF", os seriais
// que o assinador falso anexou ("%%ASSINADO:<serial>").
type fakeValidadorSignatarios struct{}

func (fakeValidadorSignatarios) Validar(_ context.Context, pdf domain.PDF) (*domain.ResultadoValidacao, error) {
	res := &domain.ResultadoValidacao{}
	for _, parte := range bytes.Split(pdf, []byte("%%ASSINADO:"))[1:] {
		res.Assinaturas = append(res.Assinaturas, domain.ResultadoAssinatura{
			Integra: true, Certificado: &domain.CertInfo{Serial: string(parte)},
		})
	}
	return res, nil
}

// hasher e cofre "de brinquedo": suficientes para testar o fluxo.
type fakeCripto struct{}

func (fakeCripto) Gerar(s string) (string, error) { return "h:" + s, nil }
func (fakeCripto) Conferir(s, h string) bool      { return h == "h:"+s }
func (fakeCripto) Selar(b []byte, senha, ctx string) ([]byte, error) {
	return append([]byte(senha+"|"+ctx+"|"), b...), nil
}
func (fakeCripto) Abrir(b []byte, senha, ctx string) ([]byte, error) {
	p := []byte(senha + "|" + ctx + "|")
	if !bytes.HasPrefix(b, p) {
		return nil, domain.ErrSenhaIncorreta
	}
	return bytes.Clone(b[len(p):]), nil
}

type fakeTokens struct{ n int }

func (f *fakeTokens) Novo() (string, []byte, error) {
	f.n++
	t := fmt.Sprintf("tok%d", f.n)
	return t, []byte("H" + t), nil
}
func (*fakeTokens) Hash(t string) []byte { return []byte("H" + t) }

type fakeAC struct{ emitidos int }

func (a *fakeAC) Emitir(_ context.Context, nome string, _ domain.CPF) (*domain.Emissao, error) {
	a.emitidos++
	agora := time.Now()
	return &domain.Emissao{Serial: fmt.Sprintf("S%d", a.emitidos), CertPEM: []byte(fmt.Sprintf("S%d", a.emitidos)), // o "PEM" falso é o próprio serial
		ChavePKCS8: []byte("CHAVE"), ValidoDe: agora, ValidoAte: agora.AddDate(1, 0, 0)}, nil
}
func (a *fakeAC) GerarLCR(_ context.Context, r []domain.Revogacao) ([]byte, error) {
	var b bytes.Buffer
	for _, x := range r {
		fmt.Fprintf(&b, "%s=%d;", x.Serial, x.Em.UnixNano())
	}
	return b.Bytes(), nil
}
func (a *fakeAC) ConsultarLCR(lcr []byte, serial string) (domain.ConsultaLCR, error) {
	var c domain.ConsultaLCR
	for _, item := range bytes.Split(lcr, []byte(";")) {
		var s string
		var ns int64
		if _, err := fmt.Sscanf(string(bytes.ReplaceAll(item, []byte("="), []byte(" "))), "%s %d", &s, &ns); err == nil && s == serial {
			c.Revogado, c.RevogadoEm = true, time.Unix(0, ns)
		}
	}
	return c, nil
}
func (a *fakeAC) Descrever([]byte) (*domain.DetalhesCertificado, error) {
	return &domain.DetalhesCertificado{}, nil
}
func (a *fakeAC) ExportarPKCS12(_, k []byte, _ string) ([]byte, error) { return k, nil }
func (a *fakeAC) RaizPEM() []byte                                      { return nil }
func (a *fakeAC) IntermediariaPEM() []byte                             { return nil }
func (a *fakeAC) CadeiaPEM() [][]byte                                  { return nil }
func (a *fakeAC) NomeEmissor() string                                  { return "AC Teste" }

type fakeAssinador struct{ chaveRecebida []byte }

func (f *fakeAssinador) Assinar(_ context.Context, p port.PedidoAssinaturaPDF) ([]byte, error) {
	f.chaveRecebida = bytes.Clone(p.ChavePKCS8)
	return append(bytes.Clone(p.PDF), "%%ASSINADO:"+string(p.CertPEM)...), nil
}

// ---------------------------------------------------------------------------

type ambiente struct {
	usuarios  *memUsuarios
	certs     *memCerts
	ac        *fakeAC
	assinador *fakeAssinador
	docs      *memDocs
	conta     *Conta
	certSvc   *Certificados
	assin     *Assinaturas
}

func novoAmbiente(t *testing.T) *ambiente {
	t.Helper()
	a := &ambiente{
		usuarios: &memUsuarios{por: map[domain.CPF]*domain.Usuario{}}, certs: &memCerts{},
		ac: &fakeAC{}, assinador: &fakeAssinador{},
	}
	var err error
	a.conta, err = NovaConta(ContaDeps{Usuarios: a.usuarios, Certificados: a.certs, Sessoes: &memSessoes{s: map[string]int64{}},
		Hasher: fakeCripto{}, Cofre: fakeCripto{}, Tokens: &fakeTokens{}, AC: a.ac})
	if err != nil {
		t.Fatal(err)
	}
	a.certSvc, _ = NovoCertificados(CertificadoDeps{Usuarios: a.usuarios, Certificados: a.certs, Hasher: fakeCripto{}, Cofre: fakeCripto{}, AC: a.ac})
	a.docs = &memDocs{certs: a.certs, chaves: map[int64][]domain.ChaveDocumento{}}
	a.assin, _ = NovoAssinaturas(AssinaturaDeps{Usuarios: a.usuarios, Certificados: a.certs, Documentos: a.docs,
		Cofre: fakeCripto{}, AC: a.ac, Assinador: a.assinador,
		Validador: fakeValidadorSignatarios{}, Cifrador: fakeCifrador{}})
	return a
}

func TestCadastroEmiteCertificadoESelaChave(t *testing.T) {
	a := novoAmbiente(t)
	ctx := context.Background()
	u, err := a.conta.Cadastrar(ctx, "  Maria   da Silva ", "529.982.247-25", "senha-boa-1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Nome != "Maria da Silva" || u.CPF != "52998224725" {
		t.Errorf("usuário normalizado errado: %+v", u)
	}
	c, err := a.certs.Atual(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(c.ChaveSelada, []byte("CHAVE")) && !bytes.HasPrefix(c.ChaveSelada, []byte("senha-boa-1|chave-usuario|")) {
		t.Error("a chave deve ser selada com a senha e o contexto do usuário")
	}
	if _, err := a.conta.Cadastrar(ctx, "Outra Pessoa", "52998224725", "senha-boa-2"); !errors.Is(err, domain.ErrCPFJaCadastrado) {
		t.Errorf("CPF duplicado: %v", err)
	}
	if _, err := a.conta.Cadastrar(ctx, "Fulano", "000", "senha-boa-1"); !errors.Is(err, domain.ErrCPFInvalido) {
		t.Errorf("CPF inválido: %v", err)
	}
}

func TestLoginESessao(t *testing.T) {
	a := novoAmbiente(t)
	ctx := context.Background()
	u, _ := a.conta.Cadastrar(ctx, "João Pereira", "111.444.777-35", "segredo123")

	if _, err := a.conta.Autenticar(ctx, "11144477735", "errada"); !errors.Is(err, domain.ErrCredenciaisInvalidas) {
		t.Errorf("senha errada: %v", err)
	}
	if _, err := a.conta.Autenticar(ctx, "529.982.247-25", "x"); !errors.Is(err, domain.ErrCredenciaisInvalidas) {
		t.Errorf("CPF inexistente deve dar a mesma resposta: %v", err)
	}
	if _, err := a.conta.Autenticar(ctx, "111.444.777-35", "segredo123"); err != nil {
		t.Fatal(err)
	}
	tok, _, _ := a.conta.IniciarSessao(ctx, u.ID)
	if id, err := a.conta.UsuarioDaSessao(ctx, tok); err != nil || id != u.ID {
		t.Errorf("sessão: %d, %v", id, err)
	}
	_ = a.conta.EncerrarSessao(ctx, tok)
	if _, err := a.conta.UsuarioDaSessao(ctx, tok); !errors.Is(err, domain.ErrSessaoInvalida) {
		t.Errorf("sessão encerrada: %v", err)
	}
}

func TestAssinarRespeitaSenhaERevogacao(t *testing.T) {
	a := novoAmbiente(t)
	ctx := context.Background()
	u, _ := a.conta.Cadastrar(ctx, "Ana Lima", "529.982.247-25", "minhasenha")
	pos, _ := domain.NovaPosicao(1, 40, 40, 300, 110)
	cmd := port.ComandoAssinar{UsuarioID: u.ID, NomeArquivo: "contrato.pdf", PDF: domain.PDF("%PDF-1.7 ..."), Posicao: pos, Senha: "errada"}

	if _, err := a.assin.Assinar(ctx, cmd); !errors.Is(err, domain.ErrSenhaIncorreta) {
		t.Fatalf("senha errada: %v", err)
	}

	cmd.Senha = "minhasenha"
	doc, err := a.assin.Assinar(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if doc.NomeArquivo != "contrato_assinado.pdf" || doc.SHA256Original == doc.SHA256Assinado {
		t.Errorf("documento: %+v", doc)
	}
	if !bytes.Equal(a.assinador.chaveRecebida, []byte("CHAVE")) {
		t.Error("o assinador deveria receber a chave aberta com a senha")
	}

	if err := a.certSvc.Revogar(ctx, u.ID, "minhasenha", domain.MotivoChaveComprometida); err != nil {
		t.Fatal(err)
	}
	if _, err := a.assin.Assinar(ctx, cmd); !errors.Is(err, domain.ErrCertificadoRevogado) {
		t.Errorf("assinar com revogado: %v", err)
	}
	if err := a.certSvc.Renovar(ctx, u.ID, "minhasenha"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.assin.Assinar(ctx, cmd); err != nil {
		t.Errorf("assinar após renovar: %v", err)
	}
	if a.ac.emitidos != 2 {
		t.Errorf("esperava 2 emissões, houve %d", a.ac.emitidos)
	}
}

type fakeValidador struct{ res domain.ResultadoValidacao }

func (f fakeValidador) Validar(context.Context, domain.PDF) (*domain.ResultadoValidacao, error) {
	r := f.res
	r.Assinaturas = append([]domain.ResultadoAssinatura(nil), f.res.Assinaturas...)
	return &r, nil
}

func TestValidacaoAplicaLCR(t *testing.T) {
	assinou := time.Now()
	certs := &memCerts{}
	revEm, motivo := assinou.Add(-time.Hour), domain.MotivoChaveComprometida
	certs.lista = []*domain.Certificado{{Serial: "AAA", RevogadoEm: &revEm, Motivo: &motivo}}

	res := domain.ResultadoValidacao{Assinaturas: []domain.ResultadoAssinatura{
		{Integra: true, CadeiaConfiavel: true, DataAssinatura: &assinou, Certificado: &domain.CertInfo{Serial: "AAA", Emissor: "AC Teste"}},
		{Integra: true, CadeiaConfiavel: true, DataAssinatura: &assinou, Certificado: &domain.CertInfo{Serial: "BBB", Emissor: "AC Teste"}},
	}}
	v := NovaValidacao(ValidacaoDeps{Certificados: certs, AC: &fakeAC{}, Validador: fakeValidador{res}})
	out, err := v.Validar(context.Background(), domain.PDF("%PDF-x"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Assinaturas[0].Valida || !out.Assinaturas[0].RevogadoAntes {
		t.Error("assinatura com certificado revogado antes deveria ser reprovada")
	}
	if !out.Assinaturas[1].Valida || !out.Assinaturas[1].EmitidoPorEstaAC {
		t.Error("assinatura limpa deveria ser aprovada")
	}
	if out.Valido {
		t.Error("o documento não é válido se alguma assinatura não for")
	}
	if out.SHA256 == "" {
		t.Error("SHA-256 do arquivo não calculado")
	}
}

func TestDocumentoCifradoAcessivelATodosOsSignatarios(t *testing.T) {
	a := novoAmbiente(t)
	ctx := context.Background()
	ana, _ := a.conta.Cadastrar(ctx, "Ana Lima", "529.982.247-25", "senha-da-ana")
	beto, _ := a.conta.Cadastrar(ctx, "Beto Souza", "111.444.777-35", "senha-do-beto")
	pos, _ := domain.NovaPosicao(1, 40, 40, 300, 110)

	// Ana assina; depois Beto assina o PDF que Ana já assinou.
	v1, err := a.assin.Assinar(ctx, port.ComandoAssinar{UsuarioID: ana.ID, NomeArquivo: "c.pdf",
		PDF: domain.PDF("%PDF-1.7 contrato"), Posicao: pos, Senha: "senha-da-ana"})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := a.assin.Assinar(ctx, port.ComandoAssinar{UsuarioID: beto.ID, NomeArquivo: "c_assinado.pdf",
		PDF: domain.PDF(v1.PDFAssinado), Posicao: pos, Senha: "senha-do-beto"})
	if err != nil {
		t.Fatal(err)
	}

	// No "banco" fica só o conteúdo cifrado.
	if bytes.Equal(a.docs.docs[1].Conteudo, v2.PDFAssinado) {
		t.Fatal("o documento foi gravado em claro")
	}
	// A versão com as duas assinaturas aparece para os dois.
	for _, u := range []*domain.Usuario{ana, beto} {
		docs, _ := a.assin.Documentos(ctx, u.ID)
		achou := false
		for _, d := range docs {
			achou = achou || d.ID == v2.ID
		}
		if !achou {
			t.Errorf("%s deveria ter acesso ao documento assinado pelos dois", u.Nome)
		}
	}
	// Ana baixa com a própria senha e recebe exatamente os bytes assinados.
	d, err := a.assin.Documento(ctx, ana.ID, v2.ID, "senha-da-ana")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.PDFAssinado, v2.PDFAssinado) {
		t.Error("o arquivo devolvido difere do assinado")
	}
	// Senha errada não abre.
	if _, err := a.assin.Documento(ctx, ana.ID, v2.ID, "errada"); !errors.Is(err, domain.ErrSenhaIncorreta) {
		t.Errorf("senha errada: %v", err)
	}
	// A primeira versão é só da Ana.
	if _, err := a.assin.Documento(ctx, beto.ID, v1.ID, "senha-do-beto"); !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("Beto não assinou a v1: %v", err)
	}
}

func TestDocumentoAlteradoNoBancoNaoEEntregue(t *testing.T) {
	a := novoAmbiente(t)
	ctx := context.Background()
	u, _ := a.conta.Cadastrar(ctx, "Ana Lima", "529.982.247-25", "senha-da-ana")
	pos, _ := domain.NovaPosicao(1, 40, 40, 300, 110)
	doc, _ := a.assin.Assinar(ctx, port.ComandoAssinar{UsuarioID: u.ID, NomeArquivo: "c.pdf",
		PDF: domain.PDF("%PDF-1.7 contrato"), Posicao: pos, Senha: "senha-da-ana"})

	a.docs.docs[0].Conteudo[3] ^= 1 // um bit trocado no armazenamento
	if _, err := a.assin.Documento(ctx, u.ID, doc.ID, "senha-da-ana"); !errors.Is(err, domain.ErrDocumentoCorrompido) {
		t.Errorf("documento adulterado deveria ser recusado: %v", err)
	}
}
