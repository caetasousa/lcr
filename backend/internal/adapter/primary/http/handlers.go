package httpadapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"lcr/internal/domain"
	"lcr/internal/port"
)

func lerJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v)
}

// ---------------------------------------------------------------------------
// Conta
// ---------------------------------------------------------------------------

type credenciais struct {
	Nome  string `json:"nome"`
	CPF   string `json:"cpf"`
	Senha string `json:"senha"`
}

func (h *Handler) iniciarSessao(w http.ResponseWriter, r *http.Request, usuarioID int64) error {
	token, expira, err := h.s.Conta.IniciarSessao(r.Context(), usuarioID)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieSessao, Value: token, Path: "/", Expires: expira,
		HttpOnly: true, Secure: h.cfg.cookieSeguro, SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (h *Handler) cadastro(w http.ResponseWriter, r *http.Request) {
	var in credenciais
	if err := lerJSON(r, &in); err != nil {
		erroJSON(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	u, err := h.s.Conta.Cadastrar(r.Context(), in.Nome, in.CPF, in.Senha)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	if err := h.iniciarSessao(w, r, u.ID); err != nil {
		responderErro(w, r, err)
		return
	}
	jsonOK(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in credenciais
	if err := lerJSON(r, &in); err != nil {
		erroJSON(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	u, err := h.s.Conta.Autenticar(r.Context(), in.CPF, in.Senha)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	if err := h.iniciarSessao(w, r, u.ID); err != nil {
		responderErro(w, r, err)
		return
	}
	jsonOK(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieSessao); err == nil {
		_ = h.s.Conta.EncerrarSessao(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSessao, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	jsonOK(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) eu(w http.ResponseWriter, r *http.Request) {
	u, err := h.s.Conta.Usuario(r.Context(), usuarioID(r))
	if err != nil {
		responderErro(w, r, err)
		return
	}
	c, det, err := h.s.Certificados.Atual(r.Context(), u.ID)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	jsonOK(w, http.StatusOK, map[string]any{
		"nome": u.Nome, "cpf": u.CPF.Formatado(), "certificado": paraCertificadoDTO(c, det, time.Now()),
	})
}

// ---------------------------------------------------------------------------
// Certificado
// ---------------------------------------------------------------------------

func (h *Handler) baixaCertificado(w http.ResponseWriter, r *http.Request) {
	c, _, err := h.s.Certificados.Atual(r.Context(), usuarioID(r))
	if err != nil {
		responderErro(w, r, err)
		return
	}
	anexo(w, "application/x-pem-file", "meu_certificado.crt", c.CertPEM)
}

type comSenha struct {
	Senha  string `json:"senha"`
	Motivo int    `json:"motivo"`
}

func (h *Handler) exportaPFX(w http.ResponseWriter, r *http.Request) {
	var in comSenha
	if err := lerJSON(r, &in); err != nil {
		erroJSON(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	pfx, err := h.s.Certificados.ExportarPFX(r.Context(), usuarioID(r), in.Senha)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	anexo(w, "application/x-pkcs12", "meu_certificado.pfx", pfx)
}

func (h *Handler) revogar(w http.ResponseWriter, r *http.Request) {
	var in comSenha
	if err := lerJSON(r, &in); err != nil {
		erroJSON(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if err := h.s.Certificados.Revogar(r.Context(), usuarioID(r), in.Senha, domain.MotivoRevogacao(in.Motivo)); err != nil {
		responderErro(w, r, err)
		return
	}
	jsonOK(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) renovar(w http.ResponseWriter, r *http.Request) {
	var in comSenha
	if err := lerJSON(r, &in); err != nil {
		erroJSON(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if err := h.s.Certificados.Renovar(r.Context(), usuarioID(r), in.Senha); err != nil {
		responderErro(w, r, err)
		return
	}
	jsonOK(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Assinatura e documentos
// ---------------------------------------------------------------------------

// lerPDF valida o upload na fronteira (tamanho e assinatura "%PDF-").
func (h *Handler) lerPDF(w http.ResponseWriter, r *http.Request) (domain.PDF, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.maxPDF+(1<<20))
	if err := r.ParseMultipartForm(h.cfg.maxPDF); err != nil {
		erroJSON(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("arquivo muito grande (máximo %d MB)", h.cfg.maxPDF>>20))
		return nil, "", false
	}
	f, hdr, err := r.FormFile("arquivo")
	if err != nil {
		erroJSON(w, http.StatusBadRequest, "envie o PDF no campo 'arquivo'")
		return nil, "", false
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		erroJSON(w, http.StatusBadRequest, "falha ao ler o arquivo")
		return nil, "", false
	}
	pdf, err := domain.NovoPDF(b)
	if err != nil {
		responderErro(w, r, err)
		return nil, "", false
	}
	return pdf, hdr.Filename, true
}

func (h *Handler) assinar(w http.ResponseWriter, r *http.Request) {
	pdf, nome, ok := h.lerPDF(w, r)
	if !ok {
		return
	}
	pagina, errP := strconv.Atoi(r.FormValue("pagina"))
	var c [4]float64
	var errC error
	for i, k := range []string{"x1", "y1", "x2", "y2"} {
		if c[i], errC = strconv.ParseFloat(r.FormValue(k), 64); errC != nil {
			break
		}
	}
	if errP != nil || errC != nil {
		responderErro(w, r, domain.ErrPosicaoInvalida)
		return
	}
	pos, err := domain.NovaPosicao(pagina, c[0], c[1], c[2], c[3])
	if err != nil {
		responderErro(w, r, err)
		return
	}
	doc, err := h.s.Assinaturas.Assinar(r.Context(), port.ComandoAssinar{
		UsuarioID: usuarioID(r), NomeArquivo: nome, PDF: pdf, Posicao: pos,
		Senha: r.FormValue("senha"), Motivo: r.FormValue("motivo"),
	})
	if err != nil {
		responderErro(w, r, err)
		return
	}
	w.Header().Set("X-Documento-Id", strconv.FormatInt(doc.ID, 10))
	w.Header().Set("X-SHA256-Original", doc.SHA256Original)
	w.Header().Set("X-SHA256-Assinado", doc.SHA256Assinado)
	anexo(w, "application/pdf", doc.NomeArquivo, doc.PDFAssinado)
}

func (h *Handler) documentos(w http.ResponseWriter, r *http.Request) {
	docs, err := h.s.Assinaturas.Documentos(r.Context(), usuarioID(r))
	if err != nil {
		responderErro(w, r, err)
		return
	}
	out := make([]documentoDTO, 0, len(docs))
	for _, d := range docs {
		out = append(out, documentoDTO{
			ID: d.ID, Nome: d.NomeArquivo, Pagina: d.Pagina, AssinadoEm: d.AssinadoEm,
			SHA256Original: d.SHA256Original, SHA256Assinado: d.SHA256Assinado, Tamanho: d.Tamanho,
		})
	}
	jsonOK(w, http.StatusOK, out)
}

func (h *Handler) baixaDocumento(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		erroJSON(w, http.StatusBadRequest, "id inválido")
		return
	}
	d, err := h.s.Assinaturas.Documento(r.Context(), usuarioID(r), id)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	anexo(w, "application/pdf", d.NomeArquivo, d.PDFAssinado)
}

// ---------------------------------------------------------------------------
// Validação e AC (públicas)
// ---------------------------------------------------------------------------

func (h *Handler) validar(w http.ResponseWriter, r *http.Request) {
	pdf, _, ok := h.lerPDF(w, r)
	if !ok {
		return
	}
	res, err := h.s.Validacao.Validar(r.Context(), pdf)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	jsonOK(w, http.StatusOK, paraValidacaoDTO(res))
}

func (h *Handler) infoAC(w http.ResponseWriter, r *http.Request) {
	info, err := h.s.AC.Info(r.Context())
	if err != nil {
		responderErro(w, r, err)
		return
	}
	out := acDTO{Raiz: detalhesDTO(&info.Raiz), Intermediaria: detalhesDTO(&info.Intermediaria), Revogados: []revogadoDTO{}}
	for _, v := range info.Revogados {
		out.Revogados = append(out.Revogados, revogadoDTO{Serial: v.Serial, Titular: v.Titular, RevogadoEm: v.Em, Motivo: v.Motivo.String()})
	}
	jsonOK(w, http.StatusOK, out)
}

func (h *Handler) baixaRaiz(w http.ResponseWriter, _ *http.Request) {
	anexo(w, "application/x-x509-ca-cert", "ac_acme_raiz_g2.crt", h.s.AC.RaizPEM())
}

func (h *Handler) baixaIntermediaria(w http.ResponseWriter, _ *http.Request) {
	anexo(w, "application/x-x509-ca-cert", "ac_acme_assinatura.crt", h.s.AC.IntermediariaPEM())
}

func (h *Handler) baixaLCR(w http.ResponseWriter, r *http.Request) {
	lcr, err := h.s.AC.LCR(r.Context())
	if err != nil {
		responderErro(w, r, err)
		return
	}
	anexo(w, "application/pkix-crl", "ac_acme.crl", lcr)
}

func anexo(w http.ResponseWriter, tipo, nome string, corpo []byte) {
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", nome))
	_, _ = w.Write(corpo)
}
