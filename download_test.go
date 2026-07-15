package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			http.Redirect(w, r, "https://buzzheavier.com/abc", http.StatusFound)
			return
		}
		switch r.URL.Path {
		case "/d/abc":
			w.Write([]byte("conteudo do capitulo"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	oldHost := downloadHost
	downloadHost = srv.URL
	defer func() { downloadHost = oldHost }()

	dest := filepath.Join(t.TempDir(), "obra", "cap-01.cbz")

	t.Run("sucesso cria arquivo e diretório", func(t *testing.T) {
		if err := downloadFile("tok123", "abc", dest); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "conteudo do capitulo" {
			t.Errorf("conteúdo = %q", data)
		}
		if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
			t.Error(".part não foi removido")
		}
	})

	t.Run("redirect (sem auth) vira erro sem criar arquivo", func(t *testing.T) {
		dest2 := filepath.Join(t.TempDir(), "cap-02.cbz")
		if err := downloadFile("token-errado", "abc", dest2); err == nil {
			t.Fatal("esperava erro")
		}
		if _, err := os.Stat(dest2); !os.IsNotExist(err) {
			t.Error("arquivo não deveria existir")
		}
	})

	t.Run("404 vira erro", func(t *testing.T) {
		if err := downloadFile("tok123", "nao-existe", filepath.Join(t.TempDir(), "x")); err == nil {
			t.Fatal("esperava erro")
		}
	})
}
