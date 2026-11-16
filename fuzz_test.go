package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Propriedade: parseSize nunca entra em panic e, se aceita a entrada,
// devolve um valor não-negativo (overflow de int64 não pode virar tamanho "válido").
func FuzzParseSize(f *testing.F) {
	for _, s := range []string{"100", "5MB", "5MiB", "1GiB", "0", "", "-5MB", "MB", "9223372036854775807", "9999999999GB"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		n, err := parseSize(s)
		if err == nil && n < 0 {
			t.Fatalf("parseSize(%q) = %d sem erro (overflow?)", s, n)
		}
	})
}

// Propriedade: parseUploadPaths nunca entra em panic e, sem erro, devolve ao menos um caminho.
func FuzzParseUploadPaths(f *testing.F) {
	dir := f.TempDir()
	file := filepath.Join(dir, "a.bin")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	for _, s := range []string{"", file, "\"" + file + "\"\r\n\n", file + "\n" + dir, "\x00", `C:\x\y`, "\n\n"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		paths, _, err := parseUploadPaths(s)
		if err == nil && len(paths) == 0 {
			t.Fatalf("parseUploadPaths(%q) sem erro e sem caminhos", s)
		}
	})
}
