package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPesadoEnfileiraEDevolve503QuandoLotado(t *testing.T) {
	h := &Handler{cfg: config{esperaMaxima: 100 * time.Millisecond}, vagas: make(chan struct{}, 1)}
	libera := make(chan struct{})
	entrou := make(chan struct{})
	lento := h.pesado(func(w http.ResponseWriter, _ *http.Request) {
		close(entrou)
		<-libera
		w.WriteHeader(http.StatusOK)
	})

	var wg sync.WaitGroup
	wg.Add(1)
	primeira := httptest.NewRecorder()
	go func() { defer wg.Done(); lento(primeira, httptest.NewRequest("POST", "/", nil)) }()
	<-entrou

	// A única vaga está ocupada: a segunda espera 100 ms e recebe 503.
	segunda := httptest.NewRecorder()
	h.pesado(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })(segunda, httptest.NewRequest("POST", "/", nil))
	if segunda.Code != http.StatusServiceUnavailable || segunda.Header().Get("Retry-After") == "" {
		t.Errorf("esperava 503 com Retry-After, veio %d", segunda.Code)
	}

	close(libera)
	wg.Wait()
	if primeira.Code != http.StatusOK {
		t.Errorf("a primeira deveria concluir: %d", primeira.Code)
	}
	// Com a vaga livre de novo, a próxima entra direto.
	terceira := httptest.NewRecorder()
	h.pesado(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })(terceira, httptest.NewRequest("POST", "/", nil))
	if terceira.Code != http.StatusOK {
		t.Errorf("a vaga deveria ter sido devolvida: %d", terceira.Code)
	}
}
