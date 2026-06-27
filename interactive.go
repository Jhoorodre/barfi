package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
)

func normalizePath(path string) string {
	if runtime.GOOS != "linux" {
		return path
	}

	// Regex para identificar caminhos do Windows, ex: C:\ ou C:/
	winPathRegex := regexp.MustCompile(`^([a-zA-Z]):[/\\](.*)$`)
	matches := winPathRegex.FindStringSubmatch(path)

	if len(matches) > 0 {
		// Tentar usar o utilitário nativo wslpath primeiro
		out, err := exec.Command("wslpath", "-u", path).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}

		// Fallback de conversão manual
		driveLetter := strings.ToLower(matches[1])
		restPath := strings.ReplaceAll(matches[2], `\`, `/`)
		return "/mnt/" + driveLetter + "/" + restPath
	}

	return path
}

func saveAndReloadCfg(mCfg *MultiConfig, cfg *Config) {
	mCfg.Profiles[mCfg.ActiveProfile] = *cfg
	path, err := defaultConfigPath()
	if err == nil {
		_ = saveConfig(path, *mCfg)
	}
}

func manageProfilesInteractive(mCfg *MultiConfig, cfg *Config) error {
	for {
		var action string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Gerenciar Perfis (Perfil atual: "+mCfg.ActiveProfile+")").
					Options(
						huh.NewOption("Trocar Perfil Ativo", "switch"),
						huh.NewOption("Editar Perfil Atual", "edit"),
						huh.NewOption("Criar Novo Perfil", "create"),
						huh.NewOption("Excluir Perfil", "delete"),
						huh.NewOption("Voltar", "back"),
					).
					Value(&action),
			),
		)
		if err := form.Run(); err != nil {
			return err
		}

		switch action {
		case "back":
			return nil
		case "switch":
			var profiles []huh.Option[string]
			for p := range mCfg.Profiles {
				profiles = append(profiles, huh.NewOption(p, p))
			}
			var selected string
			switchForm := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Selecione o Perfil Ativo").
						Options(profiles...).
						Value(&selected),
				),
			)
			if err := switchForm.Run(); err != nil {
				continue
			}
			mCfg.ActiveProfile = selected
			*cfg = mCfg.Profiles[selected]
			saveAndReloadCfg(mCfg, cfg)
		case "edit":
			var server string = cfg.Server
			var token string = cfg.Token
			var locationId string = cfg.LocationId
			var parentId string = cfg.ParentId
			var defaultNote string = cfg.DefaultNote
			var workersStr string

			if server == "" {
				server = "https://buzzheavier.com"
			}
			if cfg.Workers > 0 {
				workersStr = strconv.Itoa(cfg.Workers)
			} else {
				workersStr = "5"
			}

			editForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().Title("Endereço do Servidor").Value(&server).Validate(func(str string) error {
						if str == "" {
							return errors.New("O servidor não pode ser vazio")
						}
						return nil
					}),
					huh.NewInput().Title("Token (Opcional)").Value(&token),
					huh.NewInput().Title("ID do Bucket de Armazenamento / Location (Opcional)").Value(&locationId),
					huh.NewInput().Title("ID da Pasta de Destino / Parent (Opcional)").Value(&parentId),
					huh.NewInput().Title("Nota Padrão (Opcional)").Value(&defaultNote),
					huh.NewInput().Title("Workers").Value(&workersStr).Validate(func(str string) error {
						if str == "" {
							return nil
						}
						n, err := strconv.Atoi(str)
						if err != nil || n < 1 {
							return errors.New("Deve ser um número inteiro >= 1")
						}
						return nil
					}),
				),
			)
			if err := editForm.Run(); err != nil {
				continue
			}
			cfg.Server = server
			cfg.Token = token
			cfg.LocationId = locationId
			cfg.ParentId = parentId
			cfg.DefaultNote = defaultNote
			if workersStr != "" {
				w, _ := strconv.Atoi(workersStr)
				cfg.Workers = w
			}
			saveAndReloadCfg(mCfg, cfg)

		case "create":
			var newProfile string
			createForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().Title("Nome do novo perfil").Value(&newProfile).Validate(func(str string) error {
						if str == "" {
							return errors.New("O nome não pode ser vazio")
						}
						if _, exists := mCfg.Profiles[str]; exists {
							return errors.New("Já existe um perfil com esse nome")
						}
						return nil
					}),
				),
			)
			if err := createForm.Run(); err != nil {
				continue
			}
			mCfg.Profiles[newProfile] = Config{Server: "https://buzzheavier.com", Workers: 5}
			mCfg.ActiveProfile = newProfile
			*cfg = mCfg.Profiles[newProfile]
			saveAndReloadCfg(mCfg, cfg)

		case "delete":
			if len(mCfg.Profiles) <= 1 {
				huh.NewForm(huh.NewGroup(huh.NewNote().Title("Aviso").Description("Você não pode excluir o único perfil existente."))).Run()
				continue
			}
			var profiles []huh.Option[string]
			for p := range mCfg.Profiles {
				profiles = append(profiles, huh.NewOption(p, p))
			}
			var selected string
			deleteForm := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Selecione o perfil para excluir").
						Options(profiles...).
						Value(&selected),
				),
			)
			if err := deleteForm.Run(); err != nil {
				continue
			}
			var confirm bool
			huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Tem certeza que deseja excluir o perfil " + selected + "?").Value(&confirm))).Run()
			if confirm {
				delete(mCfg.Profiles, selected)
				if mCfg.ActiveProfile == selected {
					for p := range mCfg.Profiles {
						mCfg.ActiveProfile = p
						*cfg = mCfg.Profiles[p]
						break
					}
				}
				saveAndReloadCfg(mCfg, cfg)
			}
		}
	}
}

type fsItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IsDirectory bool   `json:"isDirectory"`
}

func listDirectory(server, token, dirID string) (items []fsItem, currentID string, err error) {
	url := server + "/api/fs"
	if dirID != "" {
		url += "/" + dirID
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("servidor retornou %d", resp.StatusCode)
	}
	var parsed struct {
		Data struct {
			ID       string   `json:"id"`
			Children []fsItem `json:"children"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, "", err
	}
	return parsed.Data.Children, parsed.Data.ID, nil
}

func createDirectory(server, token, parentID, name string) (string, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequest(http.MethodPost, server+"/api/fs/"+parentID, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("criar pasta: %s", readServerMessage(resp))
	}
	var parsed struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	return parsed.Data.ID, nil
}

func renameDirectory(server, token, dirID, name string) error {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequest(http.MethodPatch, server+"/api/fs/"+dirID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("renomear pasta: %s", readServerMessage(resp))
	}
	return nil
}

func deleteDirectory(server, token, dirID string) error {
	req, err := http.NewRequest(http.MethodDelete, server+"/api/fs/"+dirID, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("excluir pasta: %s", readServerMessage(resp))
	}
	return nil
}

func moveItem(server, token, itemID, newParentID string) error {
	body, _ := json.Marshal(map[string]string{"parentId": newParentID})
	req, err := http.NewRequest(http.MethodPut, server+"/api/fs/"+itemID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mover: %s", readServerMessage(resp))
	}
	return nil
}

func pickFolder(server, token, title, excludeID string) (string, error) {
	type crumb struct{ id, name string }

	items, rootID, err := listDirectory(server, token, "")
	if err != nil {
		return "", fmt.Errorf("listar raiz: %w", err)
	}
	path := []crumb{{rootID, "Raiz"}}
	currentItems := items

	for {
		names := make([]string, len(path))
		for i, b := range path {
			names[i] = b.name
		}
		pathStr := strings.Join(names, " / ")

		var opts []huh.Option[string]
		if len(path) > 1 {
			opts = append(opts, huh.NewOption("<< Voltar", "__back__"))
		}
		if path[len(path)-1].id != excludeID {
			opts = append(opts, huh.NewOption("Mover para aqui: "+pathStr, "__select__"))
		}
		for _, item := range currentItems {
			if item.IsDirectory && item.ID != excludeID {
				opts = append(opts, huh.NewOption("[pasta] "+item.Name, item.ID))
			}
		}

		var action string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(title + " (destino: " + pathStr + ")").
				Options(opts...).
				Value(&action),
		)).Run(); err != nil {
			return "", err
		}

		switch action {
		case "__back__":
			path = path[:len(path)-1]
			newItems, _, err := listDirectory(server, token, path[len(path)-1].id)
			if err != nil {
				eprintln("barfi:", err)
				continue
			}
			currentItems = newItems
		case "__select__":
			return path[len(path)-1].id, nil
		default:
			for _, item := range currentItems {
				if item.ID == action {
					subItems, _, err := listDirectory(server, token, item.ID)
					if err != nil {
						eprintln("barfi:", err)
						break
					}
					path = append(path, crumb{item.ID, item.Name})
					currentItems = subItems
					break
				}
			}
		}
	}
}

func browseFolders(cfg *Config, mCfg *MultiConfig) error {
	server := strings.TrimRight(cfg.Server, "/")
	if cfg.Token == "" {
		return errors.New("token obrigatório para gerenciar pastas")
	}

	type crumb struct{ id, name string }

	items, rootID, err := listDirectory(server, cfg.Token, "")
	if err != nil {
		return fmt.Errorf("listar raiz: %w", err)
	}
	path := []crumb{{rootID, "Raiz"}}
	currentItems := items

	for {
		names := make([]string, len(path))
		for i, b := range path {
			names[i] = b.name
		}
		pathStr := strings.Join(names, " / ")

		var opts []huh.Option[string]
		if len(path) > 1 {
			opts = append(opts, huh.NewOption("<< Voltar", "__back__"))
			opts = append(opts, huh.NewOption("Renomear esta pasta", "__rename__"))
			opts = append(opts, huh.NewOption("Mover esta pasta", "__move_dir__"))
			opts = append(opts, huh.NewOption("Excluir esta pasta", "__delete__"))
		}
		opts = append(opts, huh.NewOption("Selecionar como destino: "+pathStr, "__select__"))
		opts = append(opts, huh.NewOption("+ Nova pasta aqui", "__create__"))
		isFav := false
		for _, f := range cfg.Folders {
			if f.ID == path[len(path)-1].id {
				isFav = true
				break
			}
		}
		if isFav {
			opts = append(opts, huh.NewOption("Remover dos favoritos", "__unfavorite__"))
		} else {
			opts = append(opts, huh.NewOption("Adicionar aos favoritos", "__favorite__"))
		}
		for _, item := range currentItems {
			if item.IsDirectory {
				opts = append(opts, huh.NewOption("[pasta] "+item.Name, item.ID))
			} else {
				opts = append(opts, huh.NewOption("[arquivo] "+item.Name, item.ID))
			}
		}

		var action string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Pastas (atual: " + pathStr + ")").
				Options(opts...).
				Value(&action),
		)).Run(); err != nil {
			return err
		}

		switch action {
		case "__back__":
			path = path[:len(path)-1]
			newItems, _, err := listDirectory(server, cfg.Token, path[len(path)-1].id)
			if err != nil {
				eprintln("barfi:", err)
				continue
			}
			currentItems = newItems
		case "__rename__":
			var name string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title("Novo nome para '" + path[len(path)-1].name + "'").
					Value(&name).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return errors.New("nome não pode ser vazio")
						}
						return nil
					}),
			)).Run(); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}
			renamedID := path[len(path)-1].id
			if err := renameDirectory(server, cfg.Token, renamedID, name); err != nil {
				eprintln("barfi:", err)
				continue
			}
			path[len(path)-1].name = name
			for i, f := range cfg.Folders {
				if f.ID == renamedID {
					cfg.Folders[i].Name = name
					saveAndReloadCfg(mCfg, cfg)
					break
				}
			}
		case "__delete__":
			var confirm bool
			if err := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().
					Title("Excluir '" + path[len(path)-1].name + "' e todo seu conteúdo?").
					Value(&confirm),
			)).Run(); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}
			if !confirm {
				continue
			}
			if err := deleteDirectory(server, cfg.Token, path[len(path)-1].id); err != nil {
				eprintln("barfi:", err)
				continue
			}
			deletedID := path[len(path)-1].id
			if cfg.ParentId == deletedID {
				cfg.ParentId = ""
			}
			kept := cfg.Folders[:0:0]
			for _, f := range cfg.Folders {
				if f.ID != deletedID {
					kept = append(kept, f)
				}
			}
			cfg.Folders = kept
			saveAndReloadCfg(mCfg, cfg)
			path = path[:len(path)-1]
			newItems, _, err := listDirectory(server, cfg.Token, path[len(path)-1].id)
			if err != nil {
				eprintln("barfi:", err)
				continue
			}
			currentItems = newItems
		case "__move_dir__":
			cur := path[len(path)-1]
			destID, err := pickFolder(server, cfg.Token, "Mover '"+cur.name+"' para", cur.id)
			if err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					eprintln("barfi:", err)
				}
				continue
			}
			if err := moveItem(server, cfg.Token, cur.id, destID); err != nil {
				eprintln("barfi:", err)
				continue
			}
			path = path[:len(path)-1]
			newItems, _, err := listDirectory(server, cfg.Token, path[len(path)-1].id)
			if err != nil {
				eprintln("barfi:", err)
				continue
			}
			currentItems = newItems
		case "__select__":
			cfg.ParentId = path[len(path)-1].id
			saveAndReloadCfg(mCfg, cfg)
			return nil
		case "__favorite__":
			cur := path[len(path)-1]
			cfg.Folders = append(cfg.Folders, BookmarkedFolder{ID: cur.id, Name: cur.name})
			saveAndReloadCfg(mCfg, cfg)
		case "__unfavorite__":
			cur := path[len(path)-1]
			kept := cfg.Folders[:0:0]
			for _, f := range cfg.Folders {
				if f.ID != cur.id {
					kept = append(kept, f)
				}
			}
			cfg.Folders = kept
			saveAndReloadCfg(mCfg, cfg)
		case "__create__":
			var name string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewInput().Title("Nome da nova pasta").Value(&name).Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("nome não pode ser vazio")
					}
					return nil
				}),
			)).Run(); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}
			newID, err := createDirectory(server, cfg.Token, path[len(path)-1].id, name)
			if err != nil {
				eprintln("barfi:", err)
				continue
			}
			path = append(path, crumb{newID, name})
			currentItems = nil
		default:
			for _, item := range currentItems {
				if item.ID == action {
					if item.IsDirectory {
						subItems, _, err := listDirectory(server, cfg.Token, item.ID)
						if err != nil {
							eprintln("barfi:", err)
							break
						}
						path = append(path, crumb{item.ID, item.Name})
						currentItems = subItems
					} else {
						var fileAction string
						if err := huh.NewForm(huh.NewGroup(
							huh.NewSelect[string]().
								Title("Arquivo: "+item.Name).
								Options(
									huh.NewOption("Renomear", "rename"),
									huh.NewOption("Mover", "move"),
								).
								Value(&fileAction),
						)).Run(); err != nil {
							break
						}
						switch fileAction {
						case "rename":
							var name string
							if err := huh.NewForm(huh.NewGroup(
								huh.NewInput().Title("Novo nome para '" + item.Name + "'").Value(&name).Validate(func(s string) error {
									if strings.TrimSpace(s) == "" {
										return errors.New("nome não pode ser vazio")
									}
									return nil
								}),
							)).Run(); err != nil {
								break
							}
							if err := renameDirectory(server, cfg.Token, item.ID, name); err != nil {
								eprintln("barfi:", err)
							} else if newItems, _, rerr := listDirectory(server, cfg.Token, path[len(path)-1].id); rerr == nil {
								currentItems = newItems
							}
						case "move":
							destID, merr := pickFolder(server, cfg.Token, "Mover '"+item.Name+"' para", "")
							if merr != nil {
								if !errors.Is(merr, huh.ErrUserAborted) {
									eprintln("barfi:", merr)
								}
								break
							}
							if err := moveItem(server, cfg.Token, item.ID, destID); err != nil {
								eprintln("barfi:", err)
							} else if newItems, _, rerr := listDirectory(server, cfg.Token, path[len(path)-1].id); rerr == nil {
								currentItems = newItems
							}
						}
					}
					break
				}
			}
		}
	}
}

func pickUploadFolder(cfg *Config, mCfg *MultiConfig) (string, error) {
	server := strings.TrimRight(cfg.Server, "/")
	var opts []huh.Option[string]
	for _, f := range cfg.Folders {
		opts = append(opts, huh.NewOption("[pasta] "+f.Name, f.ID))
	}
	opts = append(opts, huh.NewOption("+ Criar nova pasta de série", "__create__"))
	if cfg.ParentId != "" {
		opts = append(opts, huh.NewOption("Usar pasta padrão do perfil", "__default__"))
	}
	opts = append(opts, huh.NewOption("Sem pasta específica", "__none__"))

	var action string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Enviar para qual pasta?").
			Options(opts...).
			Value(&action),
	)).Run(); err != nil {
		return "", err
	}

	switch action {
	case "__default__":
		return cfg.ParentId, nil
	case "__none__":
		return "", nil
	case "__create__":
		_, rootID, err := listDirectory(server, cfg.Token, "")
		if err != nil {
			return "", fmt.Errorf("buscar raiz: %w", err)
		}
		var name string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Nome da nova pasta de série").Value(&name).Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New("nome não pode ser vazio")
				}
				return nil
			}),
		)).Run(); err != nil {
			return "", err
		}
		parentForNew := cfg.ParentId
		if parentForNew == "" {
			parentForNew = rootID
		}
		newID, err := createDirectory(server, cfg.Token, parentForNew, name)
		if err != nil {
			return "", err
		}
		cfg.Folders = append(cfg.Folders, BookmarkedFolder{ID: newID, Name: name})
		saveAndReloadCfg(mCfg, cfg)
		return newID, nil
	default:
		return action, nil
	}
}

func runInteractiveMode(opts *cliOptions, mCfg *MultiConfig, cfg *Config) error {
	for {
		var action string
		mainMenu := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Menu Principal do Barfi (Perfil atual: "+mCfg.ActiveProfile+")").
					Options(
						huh.NewOption("Fazer Upload", "upload"),
						huh.NewOption("Gerenciar Pasta Destino", "folders"),
						huh.NewOption("Gerenciar Perfis", "profiles"),
						huh.NewOption("Sair", "exit"),
					).
					Value(&action),
			),
		)
		if err := mainMenu.Run(); err != nil {
			return err
		}

		if action == "exit" {
			return huh.ErrUserAborted
		}

		if action == "profiles" {
			if err := manageProfilesInteractive(mCfg, cfg); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
			}
			continue
		}

		if action == "folders" {
			if err := browseFolders(cfg, mCfg); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					eprintln("barfi: erro ao gerenciar pastas:", err)
				}
			}
			continue
		}

		if action == "upload" {
			var fileOrDir string
			var recursive bool
			var isDir bool

			uploadForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Qual arquivo ou pasta deseja enviar?").
						Description("Caminho completo ou relativo (aspas são ignoradas).").
						Value(&fileOrDir).
						Validate(func(str string) error {
							str = strings.Trim(str, `"' `)
							if str == "" {
								return errors.New("O caminho não pode ser vazio")
							}

							normalizedStr := normalizePath(str)
							info, err := os.Stat(normalizedStr)
							if err != nil {
								if os.IsNotExist(err) {
									return errors.New("Arquivo/Pasta não encontrado(a)")
								}
								return err
							}
							isDir = info.IsDir()
							return nil
						}),
				),
			)

			if err := uploadForm.Run(); err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					continue
				}
				return err
			}

			fileOrDir = strings.Trim(fileOrDir, `"' `)

			var extraFields []huh.Field
			if isDir {
				extraFields = append(extraFields, huh.NewConfirm().
					Title("Fazer upload do conteúdo em subpastas?").
					Description("Acessa recursivamente.").
					Value(&recursive),
				)
			}

			var note string = cfg.DefaultNote
			extraFields = append(extraFields, huh.NewInput().
				Title("Nota (Opcional)").
				Description("Máximo de 500 caracteres. Pressione Enter para manter a nota padrão do perfil.").
				Value(&note),
			)

			if len(extraFields) > 0 {
				extraForm := huh.NewForm(huh.NewGroup(extraFields...))
				if err := extraForm.Run(); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						continue
					}
					return err
				}
			}

			if cfg.Token != "" && len(cfg.Folders) > 0 {
				folderID, err := pickUploadFolder(cfg, mCfg)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						continue
					}
					return err
				}
				cfg.ParentId = folderID
			}

			opts.files = []string{normalizePath(fileOrDir)}
			opts.recursive = recursive
			opts.guestLink = ""
			opts.note = note

			return nil
		}
	}
}
