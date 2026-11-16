package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- upload: integridade e bordas ---

// Regressão crítica: as partes enviadas, concatenadas em ordem, são exatamente
// o arquivo original (offsets/tamanho da última parte corretos).
func TestRun_MultiPartReassemblesByteExact(t *testing.T) {
	fb, srv := newFakeBus(t)
	size := 2*MinPartSize + 12345 // 3 partes, a última parcial
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i * 7)
	}
	u := newTestUploader(t, content)
	u.server, u.httpClient, u.partSize, u.workers = srv.URL, srv.Client(), MinPartSize, 3

	res, err := u.run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.link, "/final-id") {
		t.Errorf("link = %q", res.link)
	}
	var got []byte
	for i := 1; i <= 3; i++ {
		got = append(got, fb.patchParts[i]...)
	}
	if sha256.Sum256(got) != sha256.Sum256(content) {
		t.Fatalf("partes reassembladas diferem do original (%d vs %d bytes)", len(got), len(content))
	}
	if len(fb.patchParts) != 3 {
		t.Errorf("partes recebidas = %d, want 3", len(fb.patchParts))
	}
}

func TestValidate_EmptyFile(t *testing.T) {
	u := newTestUploader(t, []byte("x"))
	u.fileSize = 0
	if err := u.validate(); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("err = %v", err)
	}
}

func TestValidate_NoServer(t *testing.T) {
	u := newTestUploader(t, []byte("x"))
	u.server = ""
	if err := u.validate(); err == nil || !strings.Contains(err.Error(), "server") {
		t.Errorf("err = %v", err)
	}
}

func TestReadServerMessage(t *testing.T) {
	for body, want := range map[string]string{
		`{"message":"m1"}`:            "m1",
		`{"error":"e1"}`:              "e1",
		`{"message":"m","error":"e"}`: "m",
		`texto puro  `:                "texto puro",
		``:                            "",
		`{"outro":"x"}`:               `{"outro":"x"}`,
	} {
		rec := httptest.NewRecorder()
		rec.WriteString(body)
		if got := readServerMessage(rec.Result()); got != want {
			t.Errorf("body %q → %q, want %q", body, got, want)
		}
	}
}

func TestComplete_ServerErrorAndBadJSON(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("não é json"))
	}))
	defer bad.Close()
	u2 := newTestUploader(t, []byte("x"))
	u2.server, u2.httpClient, u2.uploadId = bad.URL, bad.Client(), "i"
	if _, err := u2.complete(context.Background()); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("JSON inválido: err = %v", err)
	}

	boom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"quebrou"}`))
	}))
	defer boom.Close()
	u3 := newTestUploader(t, []byte("x"))
	u3.server, u3.httpClient, u3.uploadId = boom.URL, boom.Client(), "i"
	_, err := u3.complete(context.Background())
	se, ok := err.(*serverError)
	if !ok || se.status != 500 || se.message != "quebrou" {
		t.Errorf("err = %#v", err)
	}
}

func TestInit_ServerErrorCarriesMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"file already exists"}`))
	}))
	defer srv.Close()
	u := newTestUploader(t, []byte("x"))
	u.server, u.httpClient = srv.URL, srv.Client()
	err := u.init(context.Background())
	se, ok := err.(*serverError)
	if !ok || se.status != 409 || !strings.Contains(se.message, "already exists") {
		t.Errorf("err = %#v (o main.go depende de 'already exists' para pular o arquivo)", err)
	}
}

// Token não vai para o servidor em upload de convidado.
func TestInit_NoAuthHeaderForGuest(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"code":200,"data":{"id":"x","location":"http://l","expiresAt":"2099-01-01T00:00:00Z"}}`))
	}))
	defer srv.Close()
	u := newTestUploader(t, []byte("x"))
	u.server, u.httpClient, u.token, u.guestLinkId = srv.URL, srv.Client(), "segredo", "g1"
	if err := u.init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Errorf("Authorization vazou no upload de convidado: %q", auth)
	}
}

// --- partsize: invariantes do protocolo ---

// Constantes do protocolo BUS (espelham o servidor): mudar sem coordenar quebra uploads.
func TestProtocolConstants(t *testing.T) {
	if MinPartSize != 5<<20 || MaxPartSize != 100<<20 || MaxParts != 10000 || MaxFileSize != 1<<40 {
		t.Errorf("constantes do protocolo mudaram: %d %d %d %d", MinPartSize, MaxPartSize, MaxParts, MaxFileSize)
	}
	if headerUploadLength != "Upload-Length" || headerUploadPartNumber != "Upload-Part-Number" {
		t.Errorf("headers do protocolo mudaram")
	}
}

// Até MaxParts*MaxPartSize (~976 GiB) o tamanho automático nunca estoura MaxParts.
func TestCalcPartSize_FitsMaxPartsUpToCapacity(t *testing.T) {
	for _, size := range []int64{1, MinPartSize, MaxPartSize, MaxPartSize + 1, 50 << 30, int64(MaxParts) * MaxPartSize} {
		ps := calcPartSize(size)
		if ps < MinPartSize || ps > MaxPartSize {
			t.Errorf("size %d → part %d fora de [min,max]", size, ps)
		}
		if parts := (size + ps - 1) / ps; parts > MaxParts {
			t.Errorf("size %d → %d partes > MaxParts", size, parts)
		}
	}
}

// Documenta limite conhecido: MaxFileSize (1 TiB) é maior que MaxParts*MaxPartSize
// (~976 GiB), então arquivos entre os dois são recusados pelo validate() com
// "too many parts" em vez de subir. Se isso for corrigido, atualize este teste.
func TestKnownLimit_FileBetween976GiBAnd1TiBIsRejected(t *testing.T) {
	u := newTestUploader(t, []byte("x"))
	u.fileSize = MaxFileSize
	u.partSize = calcPartSize(u.fileSize)
	err := u.validate()
	if err == nil || !strings.Contains(err.Error(), "parts") {
		t.Fatalf("err = %v", err)
	}
	// A mensagem deve dizer qual é o tamanho máximo real (976.56 GiB), não só o nº de partes.
	if !strings.Contains(err.Error(), "976.56 GiB") {
		t.Errorf("mensagem sem o tamanho máximo efetivo: %v", err)
	}
}

// --- download: falhas não deixam lixo nem destroem arquivo existente ---

func TestDownloadFile_MidStreamFailureLeavesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("só 10 bytes"))
		// handler retorna com menos bytes que o declarado → conexão cortada
	}))
	defer srv.Close()
	old := downloadHost
	downloadHost = srv.URL
	t.Cleanup(func() { downloadHost = old })

	dest := filepath.Join(t.TempDir(), "cap.cbz")
	if err := os.WriteFile(dest, []byte("versão antiga"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := downloadFile("t", "id", dest); err == nil {
		t.Fatal("download truncado deveria falhar")
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Error(".part deveria ser removido após falha")
	}
	if b, _ := os.ReadFile(dest); string(b) != "versão antiga" {
		t.Errorf("arquivo existente foi corrompido por download falho: %q", b)
	}
}

func TestDownloadFile_SendsBearerToken(t *testing.T) {
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	old := downloadHost
	downloadHost = srv.URL
	t.Cleanup(func() { downloadHost = old })
	if err := downloadFile("meutoken", "abc123", filepath.Join(t.TempDir(), "f")); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer meutoken" || path != "/d/abc123" {
		t.Errorf("auth=%q path=%q", auth, path)
	}
}

// --- progress: helpers puros ---

func TestFormatETA(t *testing.T) {
	for in, want := range map[float64]string{
		0: "0s", 0.4: "0s", 0.5: "1s", 59: "59s", 59.6: "1m 0s", 61: "1m 1s",
		3599: "59m 59s", 3600: "1h 0m", 3725: "1h 2m",
	} {
		if got := formatETA(in); got != want {
			t.Errorf("formatETA(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateName(t *testing.T) {
	if got := truncateName("curto.bin", 20); got != "curto.bin" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("ã", 60) + ".nup"
	got := truncateName(long, 20)
	if r := []rune(got); len(r) > 20 || !strings.Contains(got, "…") {
		t.Errorf("truncateName = %q (%d runes)", got, len([]rune(got)))
	}
	if !strings.HasSuffix(got, ".nup") {
		t.Errorf("extensão deveria sobreviver à truncagem: %q", got)
	}
}

func TestMinMax64(t *testing.T) {
	if max64(1, 2) != 2 || max64(2, 1) != 2 || min64(1, 2) != 1 || min64(2, 1) != 1 || max64(-1, -2) != -1 {
		t.Error("min64/max64 incorretos")
	}
}

func TestNewProgress_Selection(t *testing.T) {
	if _, ok := newProgress(true, "x").(noopProgress); !ok {
		t.Error("--quiet deveria dar noopProgress")
	}
	// stderr de teste não é TTY → plain.
	_, _ = captureOutput(t, func() {
		if _, ok := newProgress(false, "x").(*plainProgress); !ok {
			t.Error("stderr não-TTY deveria dar plainProgress")
		}
	})
}

func TestIsTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("arquivo regular não é TTY")
	}
	f.Close()
	if isTerminal(f) {
		t.Error("arquivo fechado não é TTY")
	}
}

func TestPlainProgress_FinishReportsOutcome(t *testing.T) {
	var buf bytes.Buffer
	p := newPlainProgress(&buf, "a.bin")
	tr := newPartTracker(1, 100)
	p.Start(100, tr)
	tr.parts[0].Store(100)
	p.Finish(true)
	if !strings.Contains(buf.String(), "done: a.bin") {
		t.Errorf("saída = %q", buf.String())
	}
}
