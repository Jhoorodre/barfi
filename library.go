package main

// Lógica pura de comparação da biblioteca (local ↔ Buzzheavier).
// Separada da TUI (interactive.go) para ser testável sem terminal.

import "strings"

// libDiff é o resultado da comparação de nomes entre o disco e o servidor.
type libDiff struct {
	both       []string // presentes nos dois lados (ordem local)
	localOnly  []string // só no disco — falta enviar
	remoteOnly []string // só no Buzzheavier
}

// diffNames compara duas listas de nomes. Preserva a ordem de entrada:
// both/localOnly seguem a ordem local, remoteOnly a ordem remota.
func diffNames(local, remote []string) libDiff {
	remoteSet := make(map[string]bool, len(remote))
	for _, r := range remote {
		remoteSet[r] = true
	}
	localSet := make(map[string]bool, len(local))
	for _, l := range local {
		localSet[l] = true
	}

	var d libDiff
	for _, l := range local {
		if remoteSet[l] {
			d.both = append(d.both, l)
		} else {
			d.localOnly = append(d.localOnly, l)
		}
	}
	for _, r := range remote {
		if !localSet[r] {
			d.remoteOnly = append(d.remoteOnly, r)
		}
	}
	return d
}

// safeRemoteName informa se um nome vindo do servidor pode ser usado como
// nome de arquivo local sem escapar da pasta da obra (path traversal).
func safeRemoteName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// syncPlan é o que o sync bidirecional propõe fazer.
type syncPlan struct {
	addLocal     []string // pastas locais novas → criar/vincular no servidor
	importRemote []fsItem // pastas remotas novas → importar para a biblioteca
}

// buildSyncPlan monta o plano a partir do estado atual da biblioteca,
// das pastas locais sob o caminho base e das pastas remotas sob a pasta
// padrão do perfil. Pastas remotas já vinculadas (por nome OU por FolderID —
// cobre obra renomeada) não são reimportadas; nomes que também existem no
// disco entram por addLocal, que já vincula pasta remota homônima.
func buildSyncPlan(library []LibraryItem, localDirs []string, remoteFolders []fsItem) syncPlan {
	libNames := make(map[string]bool, len(library))
	libFolderIDs := make(map[string]bool, len(library))
	for _, item := range library {
		libNames[item.Name] = true
		if item.FolderID != "" {
			libFolderIDs[item.FolderID] = true
		}
	}
	localSet := make(map[string]bool, len(localDirs))
	for _, name := range localDirs {
		localSet[name] = true
	}

	var plan syncPlan
	for _, name := range localDirs {
		if !libNames[name] {
			plan.addLocal = append(plan.addLocal, name)
		}
	}
	for _, f := range remoteFolders {
		if !f.IsDirectory || libNames[f.Name] || libFolderIDs[f.ID] || localSet[f.Name] {
			continue
		}
		plan.importRemote = append(plan.importRemote, f)
	}
	return plan
}
