package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

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
			var libraryBasePath string = cfg.LibraryBasePath
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

			var locationField huh.Field
			if token != "" {
				if locs, err := listLocations(strings.TrimRight(server, "/"), token); err == nil && len(locs) > 0 {
					locOpts := []huh.Option[string]{huh.NewOption("Padrão (automático)", "")}
					for _, l := range locs {
						locOpts = append(locOpts, huh.NewOption(l.Name, l.ID))
					}
					locationField = huh.NewSelect[string]().
						Title("Bucket de Armazenamento").
						Description("Servidor onde os arquivos serão gravados.").
						Options(locOpts...).
						Value(&locationId)
				}
			}
			if locationField == nil {
				locationField = huh.NewInput().
					Title("ID do Bucket de Armazenamento / Location (Opcional)").
					Description("ID do servidor de armazenamento. Deixe vazio para usar o padrão.").
					Value(&locationId)
			}

			editForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().Title("Endereço do Servidor").
						Description("URL base da API. Ex: https://buzzheavier.com").
						Value(&server).Validate(func(str string) error {
						if str == "" {
							return errors.New("O servidor não pode ser vazio")
						}
						return nil
					}),
					huh.NewInput().Title("Token (Opcional)").
						Description("Seu token de autenticação. Necessário para salvar em pasta e usar favoritos.").
						Value(&token),
					locationField,
					huh.NewInput().Title("ID da Pasta de Destino / Parent (Opcional)").
						Description("ID da pasta onde os uploads serão salvos. Pode ser definido pelo navegador de pastas.").
						Value(&parentId),
					huh.NewInput().Title("Caminho Base da Biblioteca (Opcional)").
						Description("Pasta raiz onde ficam as obras. Subpastas viram entradas na biblioteca. Aceita Windows e WSL.").
						Value(&libraryBasePath),
					huh.NewInput().Title("Nota Padrão (Opcional)").
						Description("Texto exibido abaixo do link de download. Máximo 500 caracteres.").
						Value(&defaultNote),
					huh.NewInput().Title("Workers").
						Description("Quantidade de partes enviadas em paralelo. Padrão: 5.").
						Value(&workersStr).Validate(func(str string) error {
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
			cfg.LibraryBasePath = normalizePath(libraryBasePath)
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
	req, err := http.NewRequest(http.MethodPatch, server+"/api/fs/"+itemID, bytes.NewReader(body))
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

type location struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func listLocations(server, token string) ([]location, error) {
	req, err := http.NewRequest(http.MethodGet, server+"/api/locations", nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("locations: %s", readServerMessage(resp))
	}
	var parsed struct {
		Data []location `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Data, nil
}

func editFileNote(server, token, fileID, note string) error {
	body, _ := json.Marshal(map[string]string{"note": note})
	req, err := http.NewRequest(http.MethodPatch, server+"/api/fs/"+fileID, bytes.NewReader(body))
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
		return fmt.Errorf("editar nota: %s", readServerMessage(resp))
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
		opts = append(opts, huh.NewOption("← Voltar ao menu", "__cancel__"))
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
		case "__cancel__":
			return "", huh.ErrUserAborted
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
			opts = append(opts, huh.NewOption("Selecionar como destino: "+pathStr, "__select__"))
		}
		opts = append(opts, huh.NewOption("← Voltar ao menu", "__exit__"))
		opts = append(opts, huh.NewOption("+ Nova pasta aqui", "__create__"))
		if len(currentItems) > 0 {
			opts = append(opts, huh.NewOption("☑ Selecionar em lote", "__batch__"))
		}
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
		case "__exit__":
			return nil
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
			for i := range cfg.Library {
				if cfg.Library[i].FolderID == deletedID {
					cfg.Library[i].FolderID = ""
				}
			}
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
		case "__batch__":
			var batchOpts []huh.Option[string]
			for _, item := range currentItems {
				label := "[arquivo] " + item.Name
				if item.IsDirectory {
					label = "[pasta] " + item.Name
				}
				batchOpts = append(batchOpts, huh.NewOption(label, item.ID))
			}
			var selectedIDs []string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Selecionar itens em: " + pathStr).
					Description("Espaço para marcar, enter para confirmar.").
					Options(batchOpts...).
					Value(&selectedIDs),
			)).Run(); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}
			if len(selectedIDs) == 0 {
				continue
			}

			var batchAction string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title(fmt.Sprintf("%d item(s) selecionado(s)", len(selectedIDs))).
					Options(
						huh.NewOption("Mover para...", "move"),
						huh.NewOption("Excluir", "delete"),
						huh.NewOption("← Cancelar", "cancel"),
					).Value(&batchAction),
			)).Run(); err != nil || batchAction == "cancel" {
				if err != nil && !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}

			if batchAction == "move" {
				destID, err := pickFolder(server, cfg.Token, "Mover para", "")
				if err != nil {
					if !errors.Is(err, huh.ErrUserAborted) {
						eprintln("barfi:", err)
					}
					continue
				}
				selfMove := false
				for _, id := range selectedIDs {
					if id == destID {
						selfMove = true
						break
					}
				}
				if selfMove {
					eprintln("barfi: destino não pode ser um dos itens selecionados")
					continue
				}
				for _, id := range selectedIDs {
					if err := moveItem(server, cfg.Token, id, destID); err != nil {
						eprintln("barfi:", err)
					}
				}
			} else {
				var confirm bool
				if err := huh.NewForm(huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Excluir %d item(s)?", len(selectedIDs))).
						Description("Pastas serão excluídas com todo o seu conteúdo.").
						Value(&confirm),
				)).Run(); err != nil {
					if !errors.Is(err, huh.ErrUserAborted) {
						eprintln("barfi:", err)
					}
					continue
				}
				if !confirm {
					continue
				}
				deletedSet := make(map[string]bool, len(selectedIDs))
				for _, id := range selectedIDs {
					if err := deleteDirectory(server, cfg.Token, id); err != nil {
						eprintln("barfi:", err)
					} else {
						deletedSet[id] = true
					}
				}
				if len(deletedSet) > 0 {
					if deletedSet[cfg.ParentId] {
						cfg.ParentId = ""
					}
					kept := cfg.Folders[:0:0]
					for _, f := range cfg.Folders {
						if !deletedSet[f.ID] {
							kept = append(kept, f)
						}
					}
					cfg.Folders = kept
					for i := range cfg.Library {
						if deletedSet[cfg.Library[i].FolderID] {
							cfg.Library[i].FolderID = ""
						}
					}
					saveAndReloadCfg(mCfg, cfg)
				}
			}
			if newItems, _, err := listDirectory(server, cfg.Token, path[len(path)-1].id); err == nil {
				currentItems = newItems
			}

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
									huh.NewOption("Editar nota", "note"),
									huh.NewOption("Excluir", "delete"),
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
						case "note":
							var note string
							if err := huh.NewForm(huh.NewGroup(
								huh.NewInput().Title("Nota para '" + item.Name + "'").
									Description("Exibida abaixo do link de download. Deixe vazio para remover.").
									Value(&note),
							)).Run(); err != nil {
								break
							}
							if err := editFileNote(server, cfg.Token, item.ID, note); err != nil {
								eprintln("barfi:", err)
							}
						case "delete":
							var confirm bool
							if err := huh.NewForm(huh.NewGroup(
								huh.NewConfirm().Title("Excluir '" + item.Name + "'?").Value(&confirm),
							)).Run(); err != nil {
								break
							}
							if confirm {
								if err := deleteDirectory(server, cfg.Token, item.ID); err != nil {
									eprintln("barfi:", err)
								} else if newItems, _, rerr := listDirectory(server, cfg.Token, path[len(path)-1].id); rerr == nil {
									currentItems = newItems
								}
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

func syncLibrary(cfg *Config, mCfg *MultiConfig) error {
	if cfg.Token == "" {
		eprintln("barfi: token obrigatório para sincronizar biblioteca")
		return nil
	}
	basePath := normalizePath(cfg.LibraryBasePath)
	if basePath == "" {
		eprintln("barfi: caminho base da biblioteca não configurado (edite o perfil)")
		return nil
	}
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return fmt.Errorf("ler caminho base: %w", err)
	}

	existing := make(map[string]bool)
	for _, item := range cfg.Library {
		existing[item.Name] = true
	}

	var toAdd []string
	for _, e := range entries {
		if e.IsDir() && !existing[e.Name()] {
			toAdd = append(toAdd, e.Name())
		}
	}

	if len(toAdd) == 0 {
		eprintln("barfi: nenhuma obra nova encontrada em", basePath)
		return nil
	}

	var confirm bool
	preview := strings.Join(toAdd, "\n  ")
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("%d nova(s) obra(s) encontrada(s)", len(toAdd))).
			Description("Serão criadas no buzzheavier e adicionadas à biblioteca:\n  " + preview).
			Value(&confirm),
	)).Run(); err != nil {
		return err
	}
	if !confirm {
		return nil
	}

	server := strings.TrimRight(cfg.Server, "/")
	parentID := cfg.ParentId
	if parentID == "" {
		_, rootID, err := listDirectory(server, cfg.Token, "")
		if err != nil {
			return fmt.Errorf("buscar raiz: %w", err)
		}
		parentID = rootID
	}

	serverItems, _, err := listDirectory(server, cfg.Token, parentID)
	if err != nil {
		return fmt.Errorf("listar pasta destino: %w", err)
	}
	serverFolders := make(map[string]string) // name → ID
	for _, item := range serverItems {
		if item.IsDirectory {
			serverFolders[item.Name] = item.ID
		}
	}

	for _, name := range toAdd {
		var folderID string
		if id, exists := serverFolders[name]; exists {
			folderID = id
			eprintf("barfi: ↔ %s (pasta existente vinculada)\n", name)
		} else {
			id, err := createDirectory(server, cfg.Token, parentID, name)
			if err != nil {
				eprintln("barfi: criar pasta '"+name+"':", err)
				continue
			}
			folderID = id
			eprintf("barfi: ✓ %s\n", name)
		}
		cfg.Library = append(cfg.Library, LibraryItem{
			Name:      name,
			LocalPath: filepath.Join(basePath, name),
			FolderID:  folderID,
		})
	}
	saveAndReloadCfg(mCfg, cfg)
	return nil
}

func changeLibraryBasePath(cfg *Config, mCfg *MultiConfig) error {
	var newBase string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Novo caminho base da biblioteca").
			Description("Atual: " + cfg.LibraryBasePath).
			Value(&newBase).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New("caminho não pode ser vazio")
			}
			return nil
		}),
	)).Run(); err != nil {
		return err
	}
	newBase = normalizePath(newBase)

	var moveFiles bool
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Mover pastas físicas no disco?").
			Description("Se sim, cada pasta local será movida do caminho antigo para o novo.").
			Value(&moveFiles),
	)).Run(); err != nil {
		return err
	}

	oldBase := cfg.LibraryBasePath
	if oldBase != "" {
		for i, item := range cfg.Library {
			if item.LocalPath == "" {
				continue
			}
			rel, err := filepath.Rel(oldBase, item.LocalPath)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue // não é subpasta do oldBase, não toca
			}
			newPath := filepath.Join(newBase, rel)
			if moveFiles {
				if err := os.Rename(item.LocalPath, newPath); err != nil {
					eprintln("barfi: mover '"+item.Name+"':", err)
					continue
				}
			}
			cfg.Library[i].LocalPath = newPath
		}
	}
	cfg.LibraryBasePath = newBase
	saveAndReloadCfg(mCfg, cfg)
	return nil
}

// obraUpload populates opts for uploading from a library obra.
// cachedEntries may be pre-read (e.g. from a content preview); pass nil to read fresh.
// Returns nil with opts.files set on success, huh.ErrUserAborted if cancelled.
func obraUpload(cliOpts *cliOptions, cfg *Config, obra LibraryItem, cachedEntries []os.DirEntry) error {
	if obra.LocalPath == "" {
		eprintln("barfi: obra sem caminho local configurado")
		return huh.ErrUserAborted
	}
	entries := cachedEntries
	if entries == nil {
		var err error
		entries, err = os.ReadDir(obra.LocalPath)
		if err != nil {
			return fmt.Errorf("ler pasta da obra: %w", err)
		}
	}
	if len(entries) == 0 {
		eprintln("barfi: pasta da obra está vazia:", obra.LocalPath)
		return huh.ErrUserAborted
	}

	var batchMode string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Upload de: "+obra.Name).
			Description("Pasta: "+obra.LocalPath).
			Options(
				huh.NewOption("Enviar tudo", "all"),
				huh.NewOption("Selecionar capítulos/arquivos", "select"),
				huh.NewOption("← Voltar", "back"),
			).Value(&batchMode),
	)).Run(); err != nil || batchMode == "back" {
		if err != nil && !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
		return huh.ErrUserAborted
	}

	var selectedPaths []string
	if batchMode == "all" {
		for _, e := range entries {
			selectedPaths = append(selectedPaths, filepath.Join(obra.LocalPath, e.Name()))
		}
	} else {
		var entryOpts []huh.Option[string]
		for _, e := range entries {
			label := e.Name()
			if e.IsDir() {
				label = "[pasta] " + label
			}
			entryOpts = append(entryOpts, huh.NewOption(label, filepath.Join(obra.LocalPath, e.Name())))
		}
		var selected []string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Selecionar de: " + obra.Name).
				Description("Espaço para marcar, enter para confirmar.").
				Options(entryOpts...).
				Value(&selected),
		)).Run(); err != nil {
			if !errors.Is(err, huh.ErrUserAborted) {
				return err
			}
			return huh.ErrUserAborted
		}
		if len(selected) == 0 {
			eprintln("barfi: nenhum item selecionado")
			return huh.ErrUserAborted
		}
		selectedPaths = selected
	}

	note := cfg.DefaultNote
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Nota (Opcional)").
			Description("Pré-preenchida com a nota padrão do perfil. Apague para enviar sem nota.").
			Value(&note),
	)).Run(); err != nil {
		if !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
		return huh.ErrUserAborted
	}

	if obra.FolderID != "" {
		cfg.ParentId = obra.FolderID
	}
	cliOpts.files = selectedPaths
	cliOpts.recursive = false
	cliOpts.note = note
	return nil
}

func manageLibrary(cliOpts *cliOptions, cfg *Config, mCfg *MultiConfig) error {
	for {
		var menuOpts []huh.Option[string]
		for i, item := range cfg.Library {
			menuOpts = append(menuOpts, huh.NewOption("[obra] "+item.Name, fmt.Sprintf("lib:%d", i)))
		}
		for i, f := range cfg.Folders {
			menuOpts = append(menuOpts, huh.NewOption("[favorito] "+f.Name, fmt.Sprintf("fav:%d", i)))
		}
		menuOpts = append(menuOpts, huh.NewOption("+ Adicionar obra à biblioteca", "__add__"))
		menuOpts = append(menuOpts, huh.NewOption("⟳ Sincronizar com disco", "__sync__"))
		menuOpts = append(menuOpts, huh.NewOption("↪ Definir/Mudar caminho base", "__rebase__"))
		menuOpts = append(menuOpts, huh.NewOption("← Voltar", "__back__"))

		var action string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Biblioteca (Perfil: " + mCfg.ActiveProfile + ")").
				Options(menuOpts...).
				Value(&action),
		)).Run(); err != nil {
			return err
		}

		if action == "__back__" {
			return nil
		}
		if action == "__sync__" {
			if err := syncLibrary(cfg, mCfg); err != nil && !errors.Is(err, huh.ErrUserAborted) {
				eprintln("barfi:", err)
			}
			continue
		}
		if action == "__rebase__" {
			if err := changeLibraryBasePath(cfg, mCfg); err != nil && !errors.Is(err, huh.ErrUserAborted) {
				eprintln("barfi:", err)
			}
			continue
		}

		switch {
		case action == "__add__":
			item := LibraryItem{}
			var folderChoice string
			addForm := huh.NewForm(huh.NewGroup(
				huh.NewInput().Title("Nome da obra").
					Description("Nome que aparecerá na lista.").
					Value(&item.Name).Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("nome não pode ser vazio")
					}
					return nil
				}),
				huh.NewInput().Title("Caminho local (Opcional)").
					Description("Pasta no HD onde ficam os arquivos. Aceita caminhos Windows e WSL.").
					Value(&item.LocalPath),
				huh.NewSelect[string]().Title("Pasta de destino no servidor").
					Options(
						huh.NewOption("Escolher pelo navegador de pastas", "browse"),
						huh.NewOption("Usar pasta padrão do perfil", "default"),
						huh.NewOption("Nenhuma (definir depois)", "none"),
					).Value(&folderChoice),
			))
			if err := addForm.Run(); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}
			item.LocalPath = normalizePath(item.LocalPath)
			if folderChoice == "browse" {
				server := strings.TrimRight(cfg.Server, "/")
				id, err := pickFolder(server, cfg.Token, "Destino de '"+item.Name+"'", "")
				if err == nil {
					item.FolderID = id
				} else if !errors.Is(err, huh.ErrUserAborted) {
					eprintln("barfi:", err)
				}
				// ponytail: cancela o navegador mas salva a obra sem pasta (editável depois)
			} else if folderChoice == "default" {
				item.FolderID = cfg.ParentId
			}
			cfg.Library = append(cfg.Library, item)
			saveAndReloadCfg(mCfg, cfg)

		case strings.HasPrefix(action, "fav:"):
			idx, err := strconv.Atoi(action[4:])
			if err != nil || idx >= len(cfg.Folders) {
				continue
			}
			fav := cfg.Folders[idx]
			var favAction string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().Title("[favorito] "+fav.Name).
					Options(
						huh.NewOption("Remover dos favoritos", "remove"),
						huh.NewOption("← Voltar", "back"),
					).Value(&favAction),
			)).Run(); err != nil {
				continue
			}
			if favAction == "remove" {
				cfg.Folders = append(cfg.Folders[:idx], cfg.Folders[idx+1:]...)
				saveAndReloadCfg(mCfg, cfg)
			}

		case strings.HasPrefix(action, "lib:"):
			idx, err := strconv.Atoi(action[4:])
			if err != nil || idx >= len(cfg.Library) {
				continue
			}
			item := cfg.Library[idx]

			// Monta preview de conteúdo local e buzzheavier
			server := strings.TrimRight(cfg.Server, "/")
			var contentLines []string
			var localEntries []os.DirEntry // reutilizado em obraUpload para evitar leitura dupla
			if item.LocalPath != "" {
				contentLines = append(contentLines, "📁 Local ("+item.LocalPath+"):")
				if entries, err := os.ReadDir(item.LocalPath); err == nil {
					localEntries = entries
					shown := 0
					for _, e := range entries {
						if shown >= 15 {
							contentLines = append(contentLines, fmt.Sprintf("  … e mais %d item(s)", len(entries)-15))
							break
						}
						prefix := "  [arquivo] "
						if e.IsDir() {
							prefix = "  [pasta]  "
						}
						contentLines = append(contentLines, prefix+e.Name())
						shown++
					}
				} else {
					contentLines = append(contentLines, "  (erro lendo pasta: "+err.Error()+")")
				}
			}
			if item.FolderID != "" && cfg.Token != "" {
				contentLines = append(contentLines, "☁ Buzzheavier:")
				if buzzItems, _, err := listDirectory(server, cfg.Token, item.FolderID); err == nil {
					shown := 0
					for _, bi := range buzzItems {
						if shown >= 15 {
							contentLines = append(contentLines, fmt.Sprintf("  … e mais %d item(s)", len(buzzItems)-15))
							break
						}
						prefix := "  [arquivo] "
						if bi.IsDirectory {
							prefix = "  [pasta]  "
						}
						contentLines = append(contentLines, prefix+bi.Name)
						shown++
					}
				} else {
					contentLines = append(contentLines, "  (erro: "+err.Error()+")")
				}
			}
			contentDesc := strings.Join(contentLines, "\n")

			var libAction string
			var subOpts []huh.Option[string]
			if item.LocalPath != "" {
				subOpts = append(subOpts, huh.NewOption("📤 Fazer upload", "upload"))
			}
			subOpts = append(subOpts, huh.NewOption("Editar", "edit"))
			subOpts = append(subOpts, huh.NewOption("Excluir", "delete"))
			subOpts = append(subOpts, huh.NewOption("← Voltar", "back"))
			if err := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title("[obra] " + item.Name).
					Description(contentDesc).
					Options(subOpts...).
					Value(&libAction),
			)).Run(); err != nil || libAction == "back" {
				continue
			}
			switch libAction {
			case "upload":
				if err := obraUpload(cliOpts, cfg, cfg.Library[idx], localEntries); err != nil {
					if !errors.Is(err, huh.ErrUserAborted) {
						eprintln("barfi:", err)
					}
				} else {
					return nil // dispara upload no caller
				}
			case "edit":
				folderLabel := "Nenhuma"
				if cfg.Library[idx].FolderID != "" {
					folderLabel = cfg.Library[idx].FolderID
				}
				// Cópias para restaurar em caso de cancel
				origName := cfg.Library[idx].Name
				origPath := cfg.Library[idx].LocalPath
				var folderChoice string
				editForm := huh.NewForm(huh.NewGroup(
					huh.NewInput().Title("Nome da obra").Value(&cfg.Library[idx].Name).Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return errors.New("nome não pode ser vazio")
						}
						return nil
					}),
					huh.NewInput().Title("Caminho local (Opcional)").Value(&cfg.Library[idx].LocalPath),
					huh.NewSelect[string]().Title("Pasta de destino (atual: "+folderLabel+")").
						Options(
							huh.NewOption("Manter atual", "keep"),
							huh.NewOption("Escolher pelo navegador", "browse"),
							huh.NewOption("Usar pasta padrão do perfil", "default"),
							huh.NewOption("Limpar", "none"),
							huh.NewOption("← Cancelar edição", "cancel"),
						).Value(&folderChoice),
				))
				if err := editForm.Run(); err != nil || folderChoice == "cancel" {
					cfg.Library[idx].Name = origName
					cfg.Library[idx].LocalPath = origPath
					continue
				}
				cfg.Library[idx].LocalPath = normalizePath(cfg.Library[idx].LocalPath)
				switch folderChoice {
				case "browse":
					id, err := pickFolder(server, cfg.Token, "Destino de '"+cfg.Library[idx].Name+"'", "")
					if err == nil {
						cfg.Library[idx].FolderID = id
					} else if !errors.Is(err, huh.ErrUserAborted) {
						eprintln("barfi:", err)
					}
				case "default":
					cfg.Library[idx].FolderID = cfg.ParentId
				case "none":
					cfg.Library[idx].FolderID = ""
				}
				saveAndReloadCfg(mCfg, cfg)
			case "delete":
				var confirm bool
				if err := huh.NewForm(huh.NewGroup(
					huh.NewConfirm().Title("Remover '" + cfg.Library[idx].Name + "' da biblioteca?").
						Description("Não exclui nada no servidor.").Value(&confirm),
				)).Run(); err != nil {
					continue
				}
				if confirm {
					cfg.Library = append(cfg.Library[:idx], cfg.Library[idx+1:]...)
					saveAndReloadCfg(mCfg, cfg)
				}
			}
		}
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
						huh.NewOption("Gerenciar Biblioteca", "library"),
						huh.NewOption("Gerenciar Buzzheavier", "folders"),
						huh.NewOption("Gerenciar Perfis", "profiles"),
						huh.NewOption("Gerar Relatório", "report"),
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

		if action == "library" {
			if err := manageLibrary(opts, cfg, mCfg); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					eprintln("barfi: erro na biblioteca:", err)
				}
				continue
			}
			if len(opts.files) > 0 {
				return nil
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

		if action == "report" {
			filename, err := generateReport(cfg, mCfg.ActiveProfile)
			if err != nil {
				eprintln("barfi: erro ao gerar relatório:", err)
			} else {
				eprintln("barfi: relatório salvo em:", filename)
			}
			_ = huh.NewForm(huh.NewGroup(
				huh.NewNote().Title("Relatório gerado").Description(func() string {
					if err != nil {
						return "Erro: " + err.Error()
					}
					return "Salvo em: " + filename
				}()),
			)).Run()
			continue
		}

		if action == "upload" {
			var uploadType string
			if err := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title("O que deseja enviar?").
					Options(
						huh.NewOption("Arquivo ou pasta avulso", "manual"),
						huh.NewOption("Capítulo(s) de obra da biblioteca", "library"),
						huh.NewOption("← Voltar", "back"),
					).Value(&uploadType),
			)).Run(); err != nil {
				if !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}
			if uploadType == "back" {
				continue
			}

			if uploadType == "library" {
				var libOpts []huh.Option[string]
				for i, item := range cfg.Library {
					if item.LocalPath != "" {
						libOpts = append(libOpts, huh.NewOption(item.Name, strconv.Itoa(i)))
					}
				}
				if len(libOpts) == 0 {
					eprintln("barfi: nenhuma obra com caminho local. Adicione obras na biblioteca primeiro.")
					continue
				}
				libOpts = append(libOpts, huh.NewOption("← Voltar", "__back__"))

				var obraIdxStr string
				if err := huh.NewForm(huh.NewGroup(
					huh.NewSelect[string]().Title("Qual obra?").Options(libOpts...).Value(&obraIdxStr),
				)).Run(); err != nil || obraIdxStr == "__back__" {
					if err != nil && !errors.Is(err, huh.ErrUserAborted) {
						return err
					}
					continue
				}
				obraIdx, err := strconv.Atoi(obraIdxStr)
				if err != nil || obraIdx >= len(cfg.Library) {
					continue
				}
				if err := obraUpload(opts, cfg, cfg.Library[obraIdx], nil); err != nil {
					if !errors.Is(err, huh.ErrUserAborted) {
						eprintln("barfi:", err)
					}
					continue
				}
				return nil
			}

			// --- upload avulso ---
			var fileOrDir string
			var recursive bool
			var isDir bool

			if err := huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title("Arquivo ou pasta para enviar").
					Description("Caminho completo ou relativo. Aspas são ignoradas. Aceita Windows e WSL.").
					Value(&fileOrDir).
					Validate(func(str string) error {
						str = strings.Trim(str, `"' `)
						if str == "" {
							return errors.New("o caminho não pode ser vazio")
						}
						info, err := os.Stat(normalizePath(str))
						if err != nil {
							if os.IsNotExist(err) {
								return errors.New("arquivo ou pasta não encontrado(a)")
							}
							return err
						}
						isDir = info.IsDir()
						return nil
					}),
			)).Run(); err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					continue
				}
				return err
			}
			fileOrDir = strings.Trim(fileOrDir, `"' `)

			var extraFields []huh.Field
			if isDir {
				extraFields = append(extraFields, huh.NewConfirm().
					Title("Incluir subpastas?").
					Description("Acessa recursivamente todos os subdiretórios.").
					Value(&recursive),
				)
			}
			var note string = cfg.DefaultNote
			extraFields = append(extraFields, huh.NewInput().
				Title("Nota (Opcional)").
				Description("Pré-preenchida com a nota padrão. Apague para enviar sem nota.").
				Value(&note),
			)
			if err := huh.NewForm(huh.NewGroup(extraFields...)).Run(); err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					continue
				}
				return err
			}

			if cfg.Token != "" {
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
			opts.note = note
			return nil
		}
	}
}

// reportEntry holds a discovered file for the report.
type reportEntry struct {
	path string
	name string
	url  string
}

// walkBuzzheavier recursively lists all files under dirID, accumulating entries.
// pathParts tracks the folder hierarchy for display; rootName is the section label.
func walkBuzzheavier(server, token, dirID string, pathParts []string, entries *[]reportEntry) error {
	items, _, err := listDirectory(server, token, dirID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.IsDirectory {
			if err := walkBuzzheavier(server, token, item.ID, append(pathParts, item.Name), entries); err != nil {
				return err
			}
		} else {
			*entries = append(*entries, reportEntry{
				path: "/" + strings.Join(pathParts, "/"),
				name: item.Name,
				url:  server + "/" + item.ID,
			})
		}
	}
	return nil
}

// writeSection writes grouped entries (grouped by path) into sb under a section header.
func writeSection(sb *strings.Builder, header string, entries []reportEntry) {
	fmt.Fprintf(sb, "=== %s ===\n", header)
	if len(entries) == 0 {
		fmt.Fprintln(sb, "  (nenhum arquivo)")
		sb.WriteByte('\n')
		return
	}
	byPath := make(map[string][]reportEntry)
	var pathOrder []string
	seen := make(map[string]bool)
	for _, e := range entries {
		if !seen[e.path] {
			seen[e.path] = true
			pathOrder = append(pathOrder, e.path)
		}
		byPath[e.path] = append(byPath[e.path], e)
	}
	for _, p := range pathOrder {
		fmt.Fprintf(sb, "  %s/\n", p)
		for _, e := range byPath[p] {
			fmt.Fprintf(sb, "    %s | %s\n", e.name, e.url)
		}
	}
	sb.WriteByte('\n')
}

func generateReport(cfg *Config, profileName string) (string, error) {
	if cfg.Token == "" {
		return "", fmt.Errorf("token não configurado no perfil atual")
	}

	eprintln("barfi: varrendo Buzzheavier, aguarde...")

	// Build a set of library folder IDs so we can separate them from "raiz".
	libFolderIDs := make(map[string]bool)
	for _, item := range cfg.Library {
		if item.FolderID != "" {
			libFolderIDs[item.FolderID] = true
		}
	}

	// Walk each library folder separately (labeled by library item name).
	type libSection struct {
		name    string
		entries []reportEntry
	}
	var libSections []libSection
	for _, item := range cfg.Library {
		if item.FolderID == "" {
			continue
		}
		eprintln("barfi: varrendo biblioteca:", item.Name)
		var entries []reportEntry
		if err := walkBuzzheavier(cfg.Server, cfg.Token, item.FolderID, nil, &entries); err != nil {
			eprintln("barfi: aviso: erro ao varrer '"+item.Name+"':", err.Error())
			continue
		}
		libSections = append(libSections, libSection{name: item.Name, entries: entries})
	}

	// Walk root; skip top-level folders that are library folder IDs.
	eprintln("barfi: varrendo raiz...")
	rootItems, _, err := listDirectory(cfg.Server, cfg.Token, "")
	if err != nil {
		return "", fmt.Errorf("erro ao listar raiz: %w", err)
	}
	var rootEntries []reportEntry
	for _, item := range rootItems {
		if item.IsDirectory {
			if libFolderIDs[item.ID] {
				continue // already covered in library sections
			}
			if err := walkBuzzheavier(cfg.Server, cfg.Token, item.ID, []string{item.Name}, &rootEntries); err != nil {
				eprintln("barfi: aviso: erro ao varrer pasta '"+item.Name+"':", err.Error())
			}
		} else {
			rootEntries = append(rootEntries, reportEntry{
				path: "/",
				name: item.Name,
				url:  cfg.Server + "/" + item.ID,
			})
		}
	}

	// Count total files.
	total := len(rootEntries)
	for _, s := range libSections {
		total += len(s.entries)
	}

	now := time.Now()
	safeName := strings.NewReplacer(" ", "_", "/", "_", "\\", "_").Replace(profileName)
	basename := fmt.Sprintf("barfi-report-%s-%s.txt", safeName, now.Format("20060102-150405"))

	filename := basename

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Relatório Buzzheavier — Perfil: %s\n", profileName)
	fmt.Fprintf(&sb, "# Gerado em: %s\n", now.Format("02/01/2006 15:04:05"))
	fmt.Fprintf(&sb, "# Servidor: %s\n", cfg.Server)
	fmt.Fprintf(&sb, "# Total de arquivos: %d\n\n", total)

	for _, s := range libSections {
		writeSection(&sb, "Biblioteca: "+s.name, s.entries)
	}
	writeSection(&sb, "Raiz (sem biblioteca)", rootEntries)

	if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("erro ao salvar relatório: %w", err)
	}
	return filename, nil
}
