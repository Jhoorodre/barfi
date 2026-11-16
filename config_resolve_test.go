package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedConfig(t *testing.T, mCfg MultiConfig) string {
	t.Helper()
	path, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(path, mCfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func profile(cfg Config) MultiConfig {
	return MultiConfig{ActiveProfile: "Padrão", Profiles: map[string]Config{"Padrão": cfg}}
}

// Precedência documentada: flags > env > arquivo de config.
func TestResolveSettings_Precedence(t *testing.T) {
	isolateHome(t)
	seedConfig(t, profile(Config{Server: "https://file", Token: "tok-file", LocationId: "loc-file", ParentId: "par-file", Workers: 3}))

	cfg, _, err := resolveSettings(&cliOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://file" || cfg.Token != "tok-file" || cfg.LocationId != "loc-file" || cfg.Workers != 3 {
		t.Errorf("só arquivo: %+v", cfg)
	}

	t.Setenv("BARFI_SERVER", "https://env")
	t.Setenv("BARFI_TOKEN", "tok-env")
	t.Setenv("BARFI_LOCATION_ID", "loc-env")
	cfg, _, _ = resolveSettings(&cliOptions{})
	if cfg.Server != "https://env" || cfg.Token != "tok-env" || cfg.LocationId != "loc-env" {
		t.Errorf("env deveria vencer o arquivo: %+v", cfg)
	}

	cfg, _, _ = resolveSettings(&cliOptions{server: "https://flag", token: "tok-flag", locationId: "loc-flag", parentId: "par-flag", workers: 9})
	if cfg.Server != "https://flag" || cfg.Token != "tok-flag" || cfg.LocationId != "loc-flag" || cfg.ParentId != "par-flag" || cfg.Workers != 9 {
		t.Errorf("flag deveria vencer env: %+v", cfg)
	}
}

func TestResolveSettings_WorkersDefaultIs5(t *testing.T) {
	isolateHome(t)
	cfg, _, err := resolveSettings(&cliOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workers != 5 {
		t.Errorf("Workers = %d, want 5", cfg.Workers)
	}
}

// Regressão: usuário cola o ID da pasta no campo locationId. resolveSettings
// move para parentId, zera locationId e PERSISTE a correção.
func TestResolveSettings_AutoFixLocationToParent(t *testing.T) {
	isolateHome(t)
	path := seedConfig(t, profile(Config{Server: "https://s", Token: "t", LocationId: "pasta123"}))

	cfg, mCfg, err := resolveSettings(&cliOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ParentId != "pasta123" || cfg.LocationId != "" {
		t.Errorf("cfg = %+v", cfg)
	}
	if mCfg.Profiles["Padrão"].ParentId != "pasta123" {
		t.Errorf("mCfg não refletiu o fix: %+v", mCfg.Profiles["Padrão"])
	}
	onDisk, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if p := onDisk.Profiles["Padrão"]; p.ParentId != "pasta123" || p.LocationId != "" {
		t.Errorf("fix não foi salvo em disco: %+v", p)
	}
}

// O auto-fix não pode disparar quando parentId já está definido.
func TestResolveSettings_NoAutoFixWhenParentSet(t *testing.T) {
	isolateHome(t)
	seedConfig(t, profile(Config{Server: "https://s", LocationId: "loc", ParentId: "par"}))
	cfg, _, _ := resolveSettings(&cliOptions{})
	if cfg.LocationId != "loc" || cfg.ParentId != "par" {
		t.Errorf("cfg = %+v", cfg)
	}
}

// Regressão: locationId vindo só de env/flag NÃO deve ser migrado nem salvo.
func TestResolveSettings_EnvLocationNotAutoFixed(t *testing.T) {
	isolateHome(t)
	t.Setenv("BARFI_LOCATION_ID", "loc-env")
	cfg, _, _ := resolveSettings(&cliOptions{})
	if cfg.LocationId != "loc-env" || cfg.ParentId != "" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestResolveSettings_UsesActiveProfile(t *testing.T) {
	isolateHome(t)
	seedConfig(t, MultiConfig{ActiveProfile: "B", Profiles: map[string]Config{
		"Padrão": {Server: "https://a"},
		"B":      {Server: "https://b"},
	}})
	cfg, _, _ := resolveSettings(&cliOptions{})
	if cfg.Server != "https://b" {
		t.Errorf("Server = %q, want perfil ativo B", cfg.Server)
	}
}

func TestResolveSettings_CorruptConfigIsError(t *testing.T) {
	isolateHome(t)
	path, _ := defaultConfigPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte("{não é json"), 0o600)
	if _, _, err := resolveSettings(&cliOptions{}); err == nil {
		t.Fatal("config corrompido deveria dar erro (e não ser sobrescrito)")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{não é json" {
		t.Errorf("config corrompido foi sobrescrito: %q", data)
	}
}

// Migração do formato antigo (flat, pré-0.1.0) → perfil "Padrão", salvo de volta.
func TestLoadConfig_MigratesFlatFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	flat := `{"server":"https://old","token":"plain-tok","workers":4,"locationId":"L","parentId":"P"}`
	if err := os.WriteFile(path, []byte(flat), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	m, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	p := m.Profiles["Padrão"]
	if m.ActiveProfile != "Padrão" || p.Server != "https://old" || p.Token != "plain-tok" || p.Workers != 4 || p.LocationId != "L" || p.ParentId != "P" {
		t.Errorf("migração incorreta: %+v", m)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"profiles"`) {
		t.Errorf("migração não foi salva no formato novo: %s", data)
	}
	if strings.Contains(string(data), "plain-tok") {
		t.Errorf("token migrado deveria ser salvo criptografado: %s", data)
	}
}

func TestLoadConfig_EmptyObjectGetsDefaultProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_ = os.WriteFile(path, []byte(`{}`), 0o600)
	t.Setenv("HOME", t.TempDir())
	m, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Profiles["Padrão"]; !ok || m.ActiveProfile != "Padrão" {
		t.Errorf("m = %+v", m)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_ = os.WriteFile(path, []byte(`nope`), 0o600)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("JSON inválido deveria dar erro")
	}
}

func TestLoadConfig_EmptyActiveProfileDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_ = os.WriteFile(path, []byte(`{"profiles":{"X":{"server":"https://x"}}}`), 0o600)
	m, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.ActiveProfile != "Padrão" {
		t.Errorf("ActiveProfile = %q", m.ActiveProfile)
	}
}

// Token ilegível (outra máquina/usuário) é limpo com aviso, sem derrubar o load.
func TestLoadConfig_UndecryptableTokenIsCleared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_ = os.WriteFile(path, []byte(`{"activeProfile":"Padrão","profiles":{"Padrão":{"server":"https://s","token":"enc1:lixo!!"}}}`), 0o600)
	var m MultiConfig
	var err error
	_, stderr := captureOutput(t, func() { m, err = loadConfig(path) })
	if err != nil {
		t.Fatal(err)
	}
	if m.Profiles["Padrão"].Token != "" {
		t.Errorf("token deveria ser limpo: %q", m.Profiles["Padrão"].Token)
	}
	if m.Profiles["Padrão"].Server != "https://s" {
		t.Errorf("resto do perfil deveria sobreviver")
	}
	if !strings.Contains(stderr, "não pôde ser descriptografado") {
		t.Errorf("faltou aviso ao usuário: %q", stderr)
	}
}

func TestSaveConfig_EncryptsTokenAndDoesNotMutate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "c.json")
	m := profile(Config{Server: "https://s", Token: "segredo-123"})
	if err := saveConfig(path, m); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "segredo-123") {
		t.Fatalf("token em texto puro no arquivo: %s", data)
	}
	if !strings.Contains(string(data), `"token": "enc1:`) {
		t.Errorf("token sem prefixo enc1: %s", data)
	}
	if m.Profiles["Padrão"].Token != "segredo-123" {
		t.Errorf("saveConfig mutou a struct em memória: %q", m.Profiles["Padrão"].Token)
	}
}

func TestSaveConfig_RoundTripKeepsLibraryAndFolders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "c.json")
	in := profile(Config{
		DefaultNote:     "nota",
		Folders:         []BookmarkedFolder{{ID: "f1", Name: "Fav"}},
		Library:         []LibraryItem{{Name: "Obra", LocalPath: "/x/Obra", FolderID: "id9"}},
		LibraryBasePath: "/x",
	})
	if err := saveConfig(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	got := out.Profiles["Padrão"]
	if got.DefaultNote != "nota" || len(got.Folders) != 1 || got.Folders[0].ID != "f1" ||
		len(got.Library) != 1 || got.Library[0].FolderID != "id9" || got.LibraryBasePath != "/x" {
		t.Errorf("round-trip perdeu campos: %+v", got)
	}
}
