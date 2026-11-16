package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// isolateHome aponta HOME/XDG_CONFIG_HOME para diretórios temporários e limpa
// as variáveis BARFI_*. resolveSettings e loadConfig escrevem em disco
// (auto-fix, migração), então todo teste que toca config DEVE chamar isto —
// senão sobrescreve o ~/.config/barfi/config.json real (com o token).
// Não combina com t.Parallel() (usa t.Setenv).
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, k := range []string{"BARFI_SERVER", "BARFI_TOKEN", "BARFI_LOCATION_ID"} {
		t.Setenv(k, "")
	}
}

// captureOutput roda fn trocando os.Stdout/os.Stderr por arquivos temporários
// e devolve o que foi escrito em cada um.
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	dir := t.TempDir()
	so, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	se, err := os.Create(filepath.Join(dir, "err"))
	if err != nil {
		t.Fatal(err)
	}
	oldO, oldE := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = so, se
	defer func() { os.Stdout, os.Stderr = oldO, oldE }()
	fn()
	so.Close()
	se.Close()
	o, _ := os.ReadFile(so.Name())
	e, _ := os.ReadFile(se.Name())
	return string(o), string(e)
}

var (
	binOnce sync.Once
	binPath string
	binErr  error
	binDir  string
)

// barfiBin compila o binário uma única vez para todos os testes de subprocesso.
func barfiBin(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		binDir, binErr = os.MkdirTemp("", "barfi-bin-")
		if binErr != nil {
			return
		}
		binPath = filepath.Join(binDir, "barfi")
		var out bytes.Buffer
		cmd := exec.Command("go", "build", "-o", binPath, "./")
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			binErr = &buildError{err, out.String()}
		}
	})
	if binErr != nil {
		t.Fatalf("go build: %v", binErr)
	}
	return binPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + ": " + e.out }

func TestMain(m *testing.M) {
	code := m.Run()
	if binDir != "" {
		os.RemoveAll(binDir)
	}
	os.Exit(code)
}

// writeFile cria um arquivo com n bytes e devolve o caminho.
func writeFile(t *testing.T, dir, name string, n int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, bytes.Repeat([]byte("x"), n), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func contextCanceled() error { return context.Canceled }
