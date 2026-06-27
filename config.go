package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type BookmarkedFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type LibraryItem struct {
	Name      string `json:"name"`
	LocalPath string `json:"localPath,omitempty"`
	FolderID  string `json:"folderId,omitempty"`
}

type Config struct {
	Server          string             `json:"server,omitempty"`
	Token           string             `json:"token,omitempty"`
	LocationId      string             `json:"locationId,omitempty"`
	ParentId        string             `json:"parentId,omitempty"`
	Workers         int                `json:"workers,omitempty"`
	DefaultNote     string             `json:"defaultNote,omitempty"`
	Folders         []BookmarkedFolder `json:"folders,omitempty"`
	Library         []LibraryItem      `json:"library,omitempty"`
	LibraryBasePath string             `json:"libraryBasePath,omitempty"`
}

type MultiConfig struct {
	ActiveProfile string            `json:"activeProfile,omitempty"`
	Profiles      map[string]Config `json:"profiles,omitempty"`
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "barfi", "config.json"), nil
}

func loadConfig(path string) (MultiConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return MultiConfig{
				ActiveProfile: "Padrão",
				Profiles: map[string]Config{
					"Padrão": {},
				},
			}, nil
		}
		return MultiConfig{}, fmt.Errorf("read %s: %w", path, err)
	}

	var mCfg MultiConfig
	if err := json.Unmarshal(data, &mCfg); err != nil {
		return MultiConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}

	// Migration logic: If it looks like an old flat config, migrate it to Padrão
	var oldCfg Config
	_ = json.Unmarshal(data, &oldCfg)

	migrated := false
	if len(mCfg.Profiles) == 0 {
		mCfg.Profiles = make(map[string]Config)
		if oldCfg.Server != "" || oldCfg.Token != "" || oldCfg.Workers != 0 || oldCfg.LocationId != "" || oldCfg.ParentId != "" {
			mCfg.Profiles["Padrão"] = oldCfg
		} else {
			mCfg.Profiles["Padrão"] = Config{}
		}
		mCfg.ActiveProfile = "Padrão"
		migrated = true
	}

	if mCfg.ActiveProfile == "" {
		mCfg.ActiveProfile = "Padrão"
	}

	if migrated {
		_ = saveConfig(path, mCfg)
	}

	return mCfg, nil
}

func saveConfig(path string, mCfg MultiConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(mCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
