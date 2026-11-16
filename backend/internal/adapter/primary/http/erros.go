package httpadapter

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"lcr/internal/domain"
)

// statusDe mapeia cada erro de negócio para um status HTTP. Erros que não
// estão aqui são internos: o cliente recebe uma mensagem genérica e o
// detalhe vai para o log.
var statusDe = []struct {
	err    error
	status int
}{
	{domain.ErrNaoEncontrado, http.StatusNotFound},
	{domain.ErrNomeInvalido, http.StatusBadRequest},
	{domain.ErrCPFInvalido, http.StatusBadRequest},
	{domain.ErrSenhaFraca, http.StatusBadRequest},
	{domain.ErrMotivoInvalido, http.StatusBadRequest},
	{domain.ErrPosicaoInvalida, http.StatusUnprocessableEntity},
	{domain.ErrPDFInvalido, http.StatusUnprocessableEntity},
	{domain.ErrDocumentoCorrompido, http.StatusUnprocessableEntity},
	{domain.ErrCPFJaCadastrado, http.StatusConflict},
	{domain.ErrCertificadoJaRevogado, http.StatusConflict},
	{domain.ErrCertificadoAindaValido, http.StatusConflict},
	{domain.ErrCertificadoRevogado, http.StatusConflict},
	{domain.ErrCertificadoExpirado, http.StatusConflict},
	{domain.ErrCredenciaisInvalidas, http.StatusUnauthorized},
	{domain.ErrSessaoInvalida, http.StatusUnauthorized},
	{domain.ErrSenhaIncorreta, http.StatusForbidden},
}

func responderErro(w http.ResponseWriter, r *http.Request, err error) {
	for _, m := range statusDe {
		if errors.Is(err, m.err) {
			// A mensagem do erro é pensada para o usuário final (inclui
			// detalhes como "a página 5 não existe").
			erroJSON(w, m.status, err.Error())
			return
		}
	}
	slog.ErrorContext(r.Context(), "erro interno", "rota", r.URL.Path, "err", err)
	erroJSON(w, http.StatusInternalServerError, "erro interno no servidor")
}

func erroJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"erro": msg})
}

func jsonOK(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
