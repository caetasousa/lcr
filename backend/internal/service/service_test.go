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

type memDocs struct{ docs []domain.Documento }

func (m *memDocs) Salvar(_ context.Context, d *domain.Documento) error {
	d.ID = int64(len(m.docs) + 1)
	m.docs = append(m.docs, *d)
	return nil
}
func (m *memDocs) DoUsuario(context.Context, int64) ([]domain.Documento, error) { return m.docs, nil }
func (m *memDocs) Buscar(context.Context, int64, int64) (*domain.Documento, error) {
	return nil, domain.ErrNaoEncontrado
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
	return &domain.Emissao{Serial: fmt.Sprintf("S%d", a.emitidos), CertPEM: []byte("CERT " + nome),
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
	return append(bytes.Clone(p.PDF), "%%ASSINADO"...), nil
}

// ---------------------------------------------------------------------------

type ambiente struct {
	usuarios  *memUsuarios
	certs     *memCerts
	ac        *fakeAC
	assinador *fakeAssinador
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
	a.assin, _ = NovoAssinaturas(AssinaturaDeps{Usuarios: a.usuarios, Certificados: a.certs, Documentos: &memDocs{},
		Cofre: fakeCripto{}, AC: a.ac, Assinador: a.assinador})
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
