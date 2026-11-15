package httpadapter

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spa serve os arquivos do build do React; rotas desconhecidas devolvem o
// index.html para o roteador do frontend resolver (/validar, /ac, ...).
func (h *Handler) spa(w http.ResponseWriter, r *http.Request) {
	if h.cfg.diretorioEstatico == "" {
		http.NotFound(w, r)
		return
	}
	caminho := filepath.Join(h.cfg.diretorioEstatico, filepath.Clean("/"+r.URL.Path))
	if st, err := os.Stat(caminho); err == nil && !st.IsDir() {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, caminho)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(h.cfg.diretorioEstatico, "index.html"))
}
