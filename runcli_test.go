package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// runCLIIn executa runCLI in-process (HOME isolado) e devolve código + saídas.
// NUNCA chame sem arquivos/--save/--config: abriria o menu huh e travaria.
func runCLIIn(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	stdout, stderr = captureOutput(t, func() { code = runCLI(args) })
	return
}

func TestRunCLI_HelpAndVersion(t *testing.T) {
	isolateHome(t)
	code, _, stderr := runCLIIn(t, "-h")
	if code != 0 || !strings.Contains(stderr, "Uso: barfi") {
		t.Errorf("-h: code=%d stderr=%q", code, stderr)
	}
	code, stdout, _ := runCLIIn(t, "--version")
	if code != 0 || !strings.HasPrefix(stdout, "barfi ") {
		t.Errorf("--version: code=%d stdout=%q", code, stdout)
	}
}

// O --help precisa mencionar cada flag suportada.
func TestHelpDocumentsAllFlags(t *testing.T) {
	_, help := captureOutput(t, printHelp)
	for _, f := range []string{"--server", "--token", "--location-id", "--parent-id", "--guest-upload-link-id",
		"--note", "--part-size", "--workers", "--recursive", "--save", "--config", "--quiet", "--json", "--help", "--version"} {
		if !strings.Contains(help, f) {
			t.Errorf("--help não documenta %s", f)
		}
	}
}

func TestRunCLI_UsageErrorsExit2(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	f := writeFile(t, dir, "a.bin", 10)
	empty := t.TempDir()
	for name, args := range map[string][]string{
		"flag inválida":          {"--xyz"},
		"sem servidor":           {f},
		"arquivo inexistente":    {"--server", "http://127.0.0.1:1", filepath.Join(dir, "nao-existe")},
		"pasta vazia":            {"--server", "http://127.0.0.1:1", empty},
		"part-size inválido":     {"--server", "http://127.0.0.1:1", "--part-size", "abc", f},
		"part-size negativo":     {"--server", "http://127.0.0.1:1", "--part-size", "-5MB", f},
		"config ação desconhec.": {"--config", "xyz"},
	} {
		if code, _, _ := runCLIIn(t, args...); code != 2 {
			t.Errorf("%s: code = %d, want 2", name, code)
		}
	}
}

func TestRunCLI_SaveWithoutFilesPersistsEncrypted(t *testing.T) {
	isolateHome(t)
	code, _, _ := runCLIIn(t, "--server", "https://s.example", "--token", "meu-token", "-d", "pasta1", "-j", "7", "--save")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	path, _ := defaultConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "meu-token") || !strings.Contains(string(raw), "enc1:") {
		t.Errorf("token deveria estar criptografado no arquivo: %s", raw)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("modo = %v, want 0600", fi.Mode().Perm())
	}
	m, _ := loadConfig(path)
	p := m.Profiles["Padrão"]
	if p.Server != "https://s.example" || p.Token != "meu-token" || p.ParentId != "pasta1" || p.Workers != 7 {
		t.Errorf("perfil salvo = %+v", p)
	}
}

func TestRunCLI_ConfigSetShowUnset(t *testing.T) {
	isolateHome(t)
	if code, _, _ := runCLIIn(t, "--config", "set", "server", "https://x.example"); code != 0 {
		t.Fatalf("set code = %d", code)
	}
	if code, _, _ := runCLIIn(t, "--config", "set", "workers", "4"); code != 0 {
		t.Fatalf("set workers code = %d", code)
	}
	code, out, _ := runCLIIn(t, "--config", "show")
	if code != 0 {
		t.Fatalf("show code = %d", code)
	}
	var m MultiConfig
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("show não imprimiu JSON válido: %v\n%s", err, out)
	}
	if p := m.Profiles["Padrão"]; p.Server != "https://x.example" || p.Workers != 4 {
		t.Errorf("show = %+v", p)
	}
	if code, _, _ := runCLIIn(t, "--config", "unset", "server"); code != 0 {
		t.Fatalf("unset code = %d", code)
	}
	_, out, _ = runCLIIn(t, "--config", "show")
	_ = json.Unmarshal([]byte(out), &m)
	if m.Profiles["Padrão"].Server != "" || m.Profiles["Padrão"].Workers != 4 {
		t.Errorf("unset deveria limpar só server: %+v", m.Profiles["Padrão"])
	}
}

func TestRunCLI_ConfigErrorsExit2(t *testing.T) {
	isolateHome(t)
	for name, args := range map[string][]string{
		"set sem valor":      {"--config", "set", "server"},
		"set chave inválida": {"--config", "set", "foo", "bar"},
		"set workers lixo":   {"--config", "set", "workers", "abc"},
		"set workers 0":      {"--config", "set", "workers", "0"},
		"unset sem chave":    {"--config", "unset"},
		"unset inválida":     {"--config", "unset", "foo"},
	} {
		if code, _, _ := runCLIIn(t, args...); code != 2 {
			t.Errorf("%s: code = %d, want 2", name, code)
		}
	}
}

func TestRunCLI_ConfigShowCorruptExit1(t *testing.T) {
	isolateHome(t)
	path, _ := defaultConfigPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte("lixo"), 0o600)
	if code, _, _ := runCLIIn(t, "--config", "show"); code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
}

func TestRunCLI_UploadSuccess(t *testing.T) {
	isolateHome(t)
	fb, srv := newFakeBus(t)
	f := writeFile(t, t.TempDir(), "a.bin", int(MinPartSize))
	code, stdout, _ := runCLIIn(t, "--server", srv.URL, "-q", f)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.HasSuffix(strings.TrimSpace(stdout), "/final-id") {
		t.Errorf("stdout = %q", stdout)
	}
	if fb.initCalls != 1 || fb.completeCalls != 1 {
		t.Errorf("init=%d complete=%d", fb.initCalls, fb.completeCalls)
	}
}

func TestRunCLI_JSONOutput(t *testing.T) {
	isolateHome(t)
	_, srv := newFakeBus(t)
	f := writeFile(t, t.TempDir(), "a.bin", 1024)
	code, stdout, _ := runCLIIn(t, "--server", srv.URL, "-q", "--json", f)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("stdout não é JSON: %v\n%s", err, stdout)
	}
}

// Várias linhas do upload avulso viram vários args: um upload por arquivo.
func TestRunCLI_MultipleFiles(t *testing.T) {
	isolateHome(t)
	fb, srv := newFakeBus(t)
	d := t.TempDir()
	a, b := writeFile(t, d, "a.bin", 1024), writeFile(t, d, "b.bin", 2048)
	code, stdout, _ := runCLIIn(t, "--server", srv.URL, "-q", a, b)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if fb.initCalls != 2 || len(strings.Fields(stdout)) != 2 {
		t.Errorf("initCalls=%d stdout=%q", fb.initCalls, stdout)
	}
}

func TestRunCLI_DirectoryRecursiveFlag(t *testing.T) {
	isolateHome(t)
	d := t.TempDir()
	writeFile(t, d, "top.bin", 100)
	writeFile(t, d, "sub/deep.bin", 100)

	fb, srv := newFakeBus(t)
	if code, _, _ := runCLIIn(t, "--server", srv.URL, "-q", d); code != 0 || fb.initCalls != 1 {
		t.Errorf("sem -r: code=%d initCalls=%d, want 1 upload", code, fb.initCalls)
	}
	fb2, srv2 := newFakeBus(t)
	if code, _, _ := runCLIIn(t, "--server", srv2.URL, "-q", "-r", d); code != 0 || fb2.initCalls != 2 {
		t.Errorf("com -r: code=%d initCalls=%d, want 2 uploads", code, fb2.initCalls)
	}
}

func TestRunCLI_ProfileDefaultNoteAndNoteFlagPrecedence(t *testing.T) {
	isolateHome(t)
	fb, srv := newFakeBus(t)
	seedConfig(t, profile(Config{Server: srv.URL, DefaultNote: "nota do perfil"}))
	f := writeFile(t, t.TempDir(), "a.bin", 100)
	if code, _, _ := runCLIIn(t, "-q", f); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if got, _ := fb.initBody["note"].(string); got != "nota do perfil" {
		t.Errorf("note = %q, want a nota padrão do perfil", got)
	}
	fb.initBody = nil
	if code, _, _ := runCLIIn(t, "-q", "--note", "da flag", f); code != 0 {
		t.Fatal("falhou")
	}
	if got, _ := fb.initBody["note"].(string); got != "da flag" {
		t.Errorf("--note deveria vencer a nota do perfil, got %q", got)
	}
}

func TestRunCLI_PartSizeClampedToMinimum(t *testing.T) {
	isolateHome(t)
	_, srv := newFakeBus(t)
	f := writeFile(t, t.TempDir(), "a.bin", 100)
	code, _, stderr := runCLIIn(t, "--server", srv.URL, "--part-size", "1KB", f)
	if code != 0 || !strings.Contains(stderr, "abaixo do mínimo") {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
}

// Regressão: "already exists" do servidor conta como sucesso (arquivo já está lá).
func TestRunCLI_AlreadyExistsCountsAsSuccess(t *testing.T) {
	isolateHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"file already exists"}`))
	}))
	defer srv.Close()
	f := writeFile(t, t.TempDir(), "a.bin", 100)
	code, stdout, stderr := runCLIIn(t, "--server", srv.URL, f)
	if code != 0 {
		t.Fatalf("code = %d, want 0\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "já existe") || strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout=%q stderr=%q", stdout, stderr)
	}
}

// --- subprocesso: caminhos que abrem prompt huh / sinais ---

func runBin(t *testing.T, timeout time.Duration, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	bin := barfiBin(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(),
		"BARFI_SERVER=", "BARFI_TOKEN=", "BARFI_LOCATION_ID=")
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("barfi travou (timeout %v)\nstderr: %s", timeout, se.String())
	}
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, so.String(), se.String()
}

// Falha de upload em modo script (sem TTY): deve terminar com exit 1 e NÃO
// travar no prompt "tentar novamente".
func TestCLI_UploadFailureExits1WithoutHanging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"nome inválido"}`))
	}))
	defer srv.Close()
	f := writeFile(t, t.TempDir(), "a.bin", 100)
	code, _, stderr := runBin(t, 30*time.Second, "--server", srv.URL, "-q", f)
	if code != 1 {
		t.Errorf("code = %d, want 1\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "nome inválido") || !strings.Contains(stderr, "Falhas:  1") {
		t.Errorf("stderr sem resumo/mensagem do servidor: %s", stderr)
	}
}

// Arquivo vazio é recusado pelo validate() → falha do lote (exit 1).
func TestCLI_EmptyFileFails(t *testing.T) {
	_, srv := newFakeBus(t)
	f := writeFile(t, t.TempDir(), "vazio.bin", 0)
	code, _, stderr := runBin(t, 30*time.Second, "--server", srv.URL, "-q", f)
	if code != 1 || !strings.Contains(stderr, "empty") {
		t.Errorf("code=%d stderr=%s", code, stderr)
	}
}

// Lote misto: um arquivo vazio (recusado) + um ok na mesma chamada → o ok sobe,
// o resumo lista a falha, exit 1.
func TestCLI_PartialBatchFailureExits1(t *testing.T) {
	fb, srv := newFakeBus(t)
	d := t.TempDir()
	ok := writeFile(t, d, "ok.bin", 100)
	empty := writeFile(t, d, "vazio.bin", 0)
	code, stdout, stderr := runBin(t, 30*time.Second, "--server", srv.URL, "-q", empty, ok)
	if code != 1 {
		t.Errorf("code = %d, want 1\n%s", code, stderr)
	}
	if fb.initCalls != 1 || !strings.Contains(stdout, "/final-id") {
		t.Errorf("o arquivo válido deveria subir mesmo após a falha: init=%d stdout=%q", fb.initCalls, stdout)
	}
	if !strings.Contains(stderr, "Sucesso: 1") || !strings.Contains(stderr, "Falhas:  1") {
		t.Errorf("resumo do lote: %s", stderr)
	}
}

// Ctrl+C (SIGINT) durante o upload → exit 130 (não interativo).
func TestCLI_SIGINTExits130(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	defer close(release) // roda antes do srv.Close: evita travar se o teste falhar
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"id":"tok-abc","location":"` + srv.URL + `/u/tok-abc","expiresAt":"2099-01-01T00:00:00Z"}}`))
	})
	mux.HandleFunc("/u/tok-abc", func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select { // segura a parte até o cliente cancelar
		case <-r.Context().Done():
		case <-release:
		}
	})

	bin := barfiBin(t)
	f := writeFile(t, t.TempDir(), "a.bin", int(MinPartSize))
	cmd := exec.Command(bin, "--server", srv.URL, "-q", f)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	var se bytes.Buffer
	cmd.Stderr = &se
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("upload não começou")
	}
	_ = cmd.Process.Signal(syscall.SIGINT)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		ee, ok := err.(*exec.ExitError)
		if !ok || ee.ExitCode() != 130 {
			t.Errorf("err = %v, want exit 130\nstderr: %s", err, se.String())
		}
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("barfi não encerrou após SIGINT\nstderr: %s", se.String())
	}
}
