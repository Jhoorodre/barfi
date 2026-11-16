package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// --- parseUploadPaths (upload avulso com vários caminhos) ---

func TestParseUploadPaths_MultiLineQuotedAndWhitespace(t *testing.T) {
	d := t.TempDir()
	a := writeFile(t, d, "Hyrule Warriors [Jogo Base + Update v3.0.0].nup", 1)
	b := writeFile(t, d, "Zelda [v1.1.1] [v131072].nup", 1)
	// Formato real colado pelo usuário: aspas duplas, CRLF, linhas em branco no fim.
	in := "\"" + a + "\"\r\n  '" + b + "'  \r\n\r\n\n"
	paths, anyDir, err := parseUploadPaths(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != a || paths[1] != b || anyDir {
		t.Errorf("paths=%v anyDir=%v", paths, anyDir)
	}
}

func TestParseUploadPaths_DirectoryFlag(t *testing.T) {
	d := t.TempDir()
	f := writeFile(t, d, "a.bin", 1)
	_, anyDir, err := parseUploadPaths(f)
	if err != nil || anyDir {
		t.Fatalf("só arquivo: anyDir=%v err=%v", anyDir, err)
	}
	_, anyDir, err = parseUploadPaths(f + "\n" + d)
	if err != nil || !anyDir {
		t.Fatalf("arquivo + pasta: anyDir=%v err=%v", anyDir, err)
	}
}

func TestParseUploadPaths_Errors(t *testing.T) {
	d := t.TempDir()
	ok := writeFile(t, d, "ok.bin", 1)
	missing := filepath.Join(d, "nao-existe.bin")

	for _, in := range []string{"", "   ", "\n\r\n \t\n", `""`} {
		if _, _, err := parseUploadPaths(in); err == nil || !strings.Contains(err.Error(), "vazio") {
			t.Errorf("%q: err = %v, want 'vazio'", in, err)
		}
	}
	// Um caminho ruim no meio invalida o lote e a mensagem diz QUAL.
	_, _, err := parseUploadPaths(ok + "\n" + missing + "\n" + ok)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("err = %v, deveria citar %s", err, missing)
	}
}

// --- normalizePath ---

func TestNormalizePath_NonWindowsPathsUntouched(t *testing.T) {
	for _, p := range []string{"/home/x/a.bin", "rel/a.bin", "a.bin", ""} {
		if got := normalizePath(p); got != p {
			t.Errorf("normalizePath(%q) = %q", p, got)
		}
	}
}

func TestNormalizePath_WindowsDriveFallback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("conversão só em linux")
	}
	if _, err := exec.LookPath("wslpath"); err == nil {
		t.Skip("wslpath presente: usa o utilitário nativo em vez do fallback")
	}
	for in, want := range map[string]string{
		`C:\Users\foo\a.bin`:               "/mnt/c/Users/foo/a.bin",
		`D:\games\Consoles\SWITCH 2\x.nup`: "/mnt/d/games/Consoles/SWITCH 2/x.nup",
		`e:/mix/dir\file.bin`:              "/mnt/e/mix/dir/file.bin",
	} {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- helpers HTTP do file manager ---

type fsCall struct{ method, path, auth, body string }

// fakeFS registra as chamadas e responde conforme a rota.
func fakeFS(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *[]fsCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []fsCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, fsCall{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(b)})
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestListDirectory(t *testing.T) {
	srv, calls := fakeFS(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"root1","children":[{"id":"d1","name":"Pasta","isDirectory":true},{"id":"f1","name":"a.cbz","isDirectory":false}]}}`))
	})
	items, cur, err := listDirectory(srv.URL, "tok", "")
	if err != nil {
		t.Fatal(err)
	}
	if cur != "root1" || len(items) != 2 || !items[0].IsDirectory || items[1].IsDirectory || items[1].Name != "a.cbz" {
		t.Errorf("cur=%q items=%+v", cur, items)
	}
	if c := (*calls)[0]; c.method != "GET" || c.path != "/api/fs" || c.auth != "Bearer tok" {
		t.Errorf("call = %+v", c)
	}
	_, _, _ = listDirectory(srv.URL, "", "abc")
	if c := (*calls)[1]; c.path != "/api/fs/abc" || c.auth != "" {
		t.Errorf("sem token não deve mandar Authorization; call = %+v", c)
	}
}

func TestListDirectory_Errors(t *testing.T) {
	status := 404
	srv, _ := fakeFS(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
	if _, _, err := listDirectory(srv.URL, "t", "x"); !errors.Is(err, errDirNotFound) {
		t.Errorf("404: err = %v, want errDirNotFound", err)
	}
	status = 500
	if _, _, err := listDirectory(srv.URL, "t", "x"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("500: err = %v", err)
	}
	status = 200 // corpo vazio → JSON inválido
	if _, _, err := listDirectory(srv.URL, "t", "x"); err == nil {
		t.Error("JSON inválido deveria dar erro")
	}
}

func TestMutatingFSHelpers_RequestShape(t *testing.T) {
	srv, calls := fakeFS(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"novo-id"}}`))
	})
	id, err := createDirectory(srv.URL, "tok", "pai", "Minha Obra")
	if err != nil || id != "novo-id" {
		t.Fatalf("createDirectory id=%q err=%v", id, err)
	}
	if err := renameDirectory(srv.URL, "tok", "d1", "Novo Nome"); err != nil {
		t.Fatal(err)
	}
	if err := deleteDirectory(srv.URL, "tok", "d1"); err != nil {
		t.Fatal(err)
	}
	if err := moveItem(srv.URL, "tok", "f1", "destino"); err != nil {
		t.Fatal(err)
	}
	if err := editFileNote(srv.URL, "tok", "f1", "uma nota"); err != nil {
		t.Fatal(err)
	}

	type want struct{ method, path, body string }
	for i, w := range []want{
		{"POST", "/api/fs/pai", `{"name":"Minha Obra"}`},
		{"PATCH", "/api/fs/d1", `{"name":"Novo Nome"}`},
		{"DELETE", "/api/fs/d1", ``},
		{"PATCH", "/api/fs/f1", `{"parentId":"destino"}`},
		{"PATCH", "/api/fs/f1", `{"note":"uma nota"}`},
	} {
		c := (*calls)[i]
		if c.method != w.method || c.path != w.path || c.body != w.body || c.auth != "Bearer tok" {
			t.Errorf("call %d = %+v, want %+v (+Bearer tok)", i, c, w)
		}
	}
}

func TestMutatingFSHelpers_ServerErrorMessage(t *testing.T) {
	srv, _ := fakeFS(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"sem permissão"}`))
	})
	for name, err := range map[string]error{
		"criar":    func() error { _, e := createDirectory(srv.URL, "t", "p", "n"); return e }(),
		"renomear": renameDirectory(srv.URL, "t", "d", "n"),
		"excluir":  deleteDirectory(srv.URL, "t", "d"),
		"mover":    moveItem(srv.URL, "t", "i", "p"),
		"nota":     editFileNote(srv.URL, "t", "f", "n"),
	} {
		if err == nil || !strings.Contains(err.Error(), "sem permissão") {
			t.Errorf("%s: err = %v, deveria propagar a mensagem do servidor", name, err)
		}
	}
}

func TestListLocations(t *testing.T) {
	srv, calls := fakeFS(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"l1","name":"EU"},{"id":"l2","name":"US"}]}`))
	})
	locs, err := listLocations(srv.URL, "tok")
	if err != nil || len(locs) != 2 || locs[1].ID != "l2" {
		t.Fatalf("locs=%+v err=%v", locs, err)
	}
	if c := (*calls)[0]; c.path != "/api/locations" || c.auth != "Bearer tok" {
		t.Errorf("call = %+v", c)
	}
}

// --- relatório ---

// árvore: raiz R → {a.cbz, Sub → {b.cbz}}
func reportTree(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/fs/R":
		_, _ = w.Write([]byte(`{"data":{"id":"R","children":[{"id":"fa","name":"a.cbz","isDirectory":false},{"id":"S","name":"Sub","isDirectory":true}]}}`))
	case "/api/fs/S":
		_, _ = w.Write([]byte(`{"data":{"id":"S","children":[{"id":"fb","name":"b.cbz","isDirectory":false}]}}`))
	default:
		http.NotFound(w, r)
	}
}

func TestWalkBuzzheavier(t *testing.T) {
	srv, _ := fakeFS(t, reportTree)
	var entries []reportEntry
	if err := walkBuzzheavier(srv.URL, "tok", "R", nil, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].name != "a.cbz" || entries[0].path != "/" || entries[0].url != srv.URL+"/fa" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1].name != "b.cbz" || entries[1].path != "/Sub" || entries[1].url != srv.URL+"/fb" {
		t.Errorf("entries[1] = %+v", entries[1])
	}
}

func TestWalkBuzzheavier_PropagatesError(t *testing.T) {
	srv, _ := fakeFS(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	var entries []reportEntry
	if err := walkBuzzheavier(srv.URL, "t", "nada", nil, &entries); !errors.Is(err, errDirNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestWriteSection(t *testing.T) {
	var sb strings.Builder
	writeSection(&sb, "Vazia", nil)
	if got := sb.String(); got != "=== Vazia ===\n  (nenhum arquivo)\n\n" {
		t.Errorf("vazia = %q", got)
	}
	sb.Reset()
	writeSection(&sb, "Obra", []reportEntry{
		{path: "/", name: "a", url: "u/a"},
		{path: "/Sub", name: "b", url: "u/b"},
		{path: "/", name: "c", url: "u/c"}, // agrupa com "/" mesmo fora de ordem
	})
	want := "=== Obra ===\n  //\n    a | u/a\n    c | u/c\n  /Sub/\n    b | u/b\n\n"
	if got := sb.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestGenerateReport_RequiresToken(t *testing.T) {
	if _, err := generateReport(&Config{Server: "https://x"}, "Padrão"); err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("err = %v", err)
	}
}

func TestGenerateReport_WritesFile(t *testing.T) {
	srv, _ := fakeFS(t, reportTree)
	cfg := &Config{Server: srv.URL, Token: "tok", Library: []LibraryItem{
		{Name: "Obra Um", FolderID: "R"},
		{Name: "Sem pasta"},                       // sem FolderID: ignorada
		{Name: "Quebrada", FolderID: "naoexiste"}, // erro de varredura: só aviso
	}}
	var name string
	var err error
	_, _ = captureOutput(t, func() { name, err = generateReport(cfg, "Meu Perfil/x") })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(name) })
	if !strings.Contains(filepath.Base(name), "barfi-report-Meu-Perfil-x-") {
		t.Errorf("nome do arquivo = %s (espaço e / do perfil devem virar -)", name)
	}
	data, _ := os.ReadFile(name)
	got := string(data)
	for _, want := range []string{"Perfil: Meu Perfil/x", "Total de arquivos: 2", "=== Obra Um ===", "a.cbz | " + srv.URL + "/fa", "b.cbz | " + srv.URL + "/fb"} {
		if !strings.Contains(got, want) {
			t.Errorf("relatório sem %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Sem pasta") || strings.Contains(got, "Quebrada") {
		t.Errorf("obras sem pasta/quebradas não devem virar seção:\n%s", got)
	}
}

// Sanidade do contrato JSON do fsItem (campo isDirectory).
func TestFsItemJSONContract(t *testing.T) {
	var it fsItem
	if err := json.Unmarshal([]byte(`{"id":"1","name":"n","isDirectory":true}`), &it); err != nil || !it.IsDirectory || it.ID != "1" {
		t.Errorf("it=%+v err=%v", it, err)
	}
}
