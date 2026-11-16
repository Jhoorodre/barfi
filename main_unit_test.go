package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseFlags_FilesAndFlags(t *testing.T) {
	opts, err := parseFlags([]string{"-d", "dir1", "-j", "3", "-q", "--note", "oi", "a.bin", "b.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.parentId != "dir1" || opts.workers != 3 || !opts.quiet || opts.note != "oi" {
		t.Errorf("flags mal parseadas: %+v", opts)
	}
	if !reflect.DeepEqual(opts.files, []string{"a.bin", "b.bin"}) {
		t.Errorf("files = %v", opts.files)
	}
}

// Regressão: o parser do Go para na primeira não-flag; flags depois dos
// arquivos viram nomes de arquivo. O --help documenta flags antes dos arquivos.
func TestParseFlags_FlagsAfterFilesAreNotParsed(t *testing.T) {
	opts, err := parseFlags([]string{"a.bin", "-d", "dir1"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.parentId != "" {
		t.Errorf("parentId = %q, flag depois de arquivo não deveria ser parseada", opts.parentId)
	}
	if !reflect.DeepEqual(opts.files, []string{"a.bin", "-d", "dir1"}) {
		t.Errorf("files = %v", opts.files)
	}
}

func TestParseFlags_ConfigModeUsesConfigArgs(t *testing.T) {
	opts, err := parseFlags([]string{"--config", "set", "server", "https://x"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.configAction != "set" {
		t.Fatalf("configAction = %q", opts.configAction)
	}
	if len(opts.files) != 0 {
		t.Errorf("em modo --config os posicionais não são arquivos: %v", opts.files)
	}
	if !reflect.DeepEqual(opts.configArgs, []string{"server", "https://x"}) {
		t.Errorf("configArgs = %v", opts.configArgs)
	}
}

func TestParseFlags_ShortAndLongAliases(t *testing.T) {
	for _, args := range [][]string{{"-l", "L1", "-r", "-h"}, {"--location-id", "L1", "--recursive", "--help"}} {
		opts, err := parseFlags(args)
		if err != nil {
			t.Fatal(err)
		}
		if opts.locationId != "L1" || !opts.recursive || !opts.showHelp {
			t.Errorf("%v => %+v", args, opts)
		}
	}
}

func TestParseFlags_UnknownFlag(t *testing.T) {
	var err error
	_, _ = captureOutput(t, func() { _, err = parseFlags([]string{"--nao-existe"}) })
	if err == nil {
		t.Fatal("flag desconhecida deveria dar erro")
	}
}

func TestSetConfigField(t *testing.T) {
	var c Config
	for k, v := range map[string]string{"server": "S", "token": "T", "locationId": "L", "parentId": "P"} {
		if err := setConfigField(&c, k, v); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
	if c.Server != "S" || c.Token != "T" || c.LocationId != "L" || c.ParentId != "P" {
		t.Errorf("campos não aplicados: %+v", c)
	}
	if err := setConfigField(&c, "workers", "8"); err != nil || c.Workers != 8 {
		t.Errorf("workers=8: err=%v workers=%d", err, c.Workers)
	}
	if err := setConfigField(&c, "workers", ""); err != nil || c.Workers != 0 {
		t.Errorf("workers vazio deve zerar: err=%v workers=%d", err, c.Workers)
	}
}

func TestSetConfigField_Errors(t *testing.T) {
	var c Config
	for _, tc := range []struct{ key, val, want string }{
		{"workers", "abc", "integer"},
		{"workers", "0", ">= 1"},
		{"workers", "-2", ">= 1"},
		{"naoexiste", "x", "unknown config key"},
	} {
		err := setConfigField(&c, tc.key, tc.val)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s=%q: err=%v, want contendo %q", tc.key, tc.val, err, tc.want)
		}
	}
	if c.Workers != 0 {
		t.Errorf("valor inválido não pode alterar o config: workers=%d", c.Workers)
	}
}

func TestExpandFiles(t *testing.T) {
	root := t.TempDir()
	a := writeFile(t, root, "a.bin", 1)
	b := writeFile(t, root, "sub/b.bin", 1)
	c := writeFile(t, root, "sub/deep/c.bin", 1)

	sorted := func(s []string, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(s)
		return s
	}

	if got := sorted(expandFiles([]string{a}, false)); !reflect.DeepEqual(got, []string{a}) {
		t.Errorf("arquivo avulso: %v", got)
	}
	// Sem -r: só os arquivos do topo, subpastas ignoradas.
	if got := sorted(expandFiles([]string{root}, false)); !reflect.DeepEqual(got, []string{a}) {
		t.Errorf("flat = %v, want só %v", got, a)
	}
	// Com -r: tudo.
	want := []string{a, b, c}
	sort.Strings(want)
	if got := sorted(expandFiles([]string{root}, true)); !reflect.DeepEqual(got, want) {
		t.Errorf("recursivo = %v, want %v", got, want)
	}
	// Mistura de arquivo + pasta (caso do upload avulso com várias linhas).
	got := sorted(expandFiles([]string{c, filepath.Join(root, "sub")}, false))
	wantMix := []string{b, c}
	sort.Strings(wantMix)
	if !reflect.DeepEqual(got, wantMix) {
		t.Errorf("mix = %v, want %v", got, wantMix)
	}
}

func TestExpandFiles_MissingPath(t *testing.T) {
	_, err := expandFiles([]string{filepath.Join(t.TempDir(), "nao-existe")}, false)
	if err == nil || !strings.Contains(err.Error(), "erro acessando") {
		t.Fatalf("err = %v", err)
	}
}

func TestExpandFiles_EmptyDir(t *testing.T) {
	got, err := expandFiles([]string{t.TempDir()}, true)
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestFilepathBase(t *testing.T) {
	for in, want := range map[string]string{
		"a.bin":                 "a.bin",
		"/x/y/a.bin":            "a.bin",
		`C:\Users\foo\a b.nup`:  "a b.nup",
		`D:\games\x/y\jogo.nup`: "jogo.nup",
		"dir/":                  "",
		"":                      "",
	} {
		if got := filepathBase(in); got != want {
			t.Errorf("filepathBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatError_ExitCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{errExpired, 1},
		{errPartTooLarge, 1},
		{errors.New("qualquer"), 1},
		{&serverError{status: 500, message: "x"}, 1},
	} {
		_, _ = captureOutput(t, func() {
			if got := formatError(tc.err); got != tc.want {
				t.Errorf("formatError(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
	_, _ = captureOutput(t, func() {
		if got := formatError(errors.Join(errors.New("x"), contextCanceled())); got != 130 {
			t.Errorf("context.Canceled deveria dar 130, got %d", got)
		}
	})
}

// Regressão: wrap com %w deve continuar sendo reconhecido (errors.Is).
func TestFormatError_WrappedSentinel(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		formatError(errors.Join(errors.New("ctx"), errExpired))
	})
	if !strings.Contains(stderr, "expired") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestServerErrorMessage(t *testing.T) {
	e := &serverError{status: 409, message: "file already exists"}
	if !strings.Contains(e.Error(), "409") || !strings.Contains(e.Error(), "already exists") {
		t.Errorf("Error() = %q", e.Error())
	}
}
