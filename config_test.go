package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	mCfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if mCfg.ActiveProfile != "Padrão" {
		t.Fatalf("expected active profile to be Padrão, got %s", mCfg.ActiveProfile)
	}
	if _, exists := mCfg.Profiles["Padrão"]; !exists {
		t.Fatalf("expected Padrão profile to exist")
	}
}

func TestSaveConfig_CreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "barfi")
	path := filepath.Join(dir, "config.json")
	mCfg := MultiConfig{
		ActiveProfile: "Padrão",
		Profiles: map[string]Config{
			"Padrão": {
				Server:     "https://bus.example.com",
				Token:      "tok",
				LocationId: "loc",
				ParentId:   "par",
				Workers:    5,
			},
		},
	}
	if err := saveConfig(path, mCfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", di.Mode().Perm())
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", fi.Mode().Perm())
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	gotP, wantP := loaded.Profiles["Padrão"], mCfg.Profiles["Padrão"]
	if loaded.ActiveProfile != mCfg.ActiveProfile || gotP.Server != wantP.Server || gotP.Token != wantP.Token || gotP.Workers != wantP.Workers {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", loaded, mCfg)
	}
}

func TestSaveConfig_OmitEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "barfi")
	path := filepath.Join(dir, "config.json")
	mCfg := MultiConfig{
		ActiveProfile: "Padrão",
		Profiles: map[string]Config{
			"Padrão": {Server: "https://bus.example.com"},
		},
	}
	if err := saveConfig(path, mCfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(data)
	for _, forbidden := range []string{`"token"`, `"locationId"`, `"parentId"`, `"workers"`} {
		if strings.Contains(got, forbidden) {
			t.Errorf("JSON should omit %s on empty value, got: %s", forbidden, got)
		}
	}
}

func TestSaveConfig_PreservesExistingFieldsViaResolve(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "barfi")
	path := filepath.Join(dir, "config.json")

	mCfg := MultiConfig{
		ActiveProfile: "Padrão",
		Profiles: map[string]Config{
			"Padrão": {Server: "https://a", Token: "tok"},
		},
	}
	if err := saveConfig(path, mCfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded.Profiles["Padrão"]
	cfg.ParentId = "abc"
	loaded.Profiles["Padrão"] = cfg
	if err := saveConfig(path, loaded); err != nil {
		t.Fatal(err)
	}

	final, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	finalCfg := final.Profiles["Padrão"]
	if finalCfg.Server != "https://a" {
		t.Errorf("Server = %q, want %q", finalCfg.Server, "https://a")
	}
	if finalCfg.Token != "tok" {
		t.Errorf("Token = %q, want %q", finalCfg.Token, "tok")
	}
	if finalCfg.ParentId != "abc" {
		t.Errorf("ParentId = %q, want %q", finalCfg.ParentId, "abc")
	}
}
