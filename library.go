package main

// Lógica pura de comparação da biblioteca (local ↔ Buzzheavier).
// Separada da TUI (interactive.go) para ser testável sem terminal.

import (
	"path/filepath"
	"strings"
)

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
		remoteSet[normKey(r)] = true
	}
	localSet := make(map[string]bool, len(local))
	for _, l := range local {
		localSet[normKey(l)] = true
	}

	var d libDiff
	for _, l := range local {
		if remoteSet[normKey(l)] {
			d.both = append(d.both, l)
		} else {
			d.localOnly = append(d.localOnly, l)
		}
	}
	for _, r := range remote {
		if !localSet[normKey(r)] {
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

// normalizeFSName reduz um nome à forma segura para gravar em disco em
// qualquer sistema suportado: remove os caracteres proibidos no Windows
// (< > : " / \ | ? *), caracteres de controle (0x00–0x1F, 0x7F — inválidos
// ou perigosos em qualquer SO, incluindo Linux) e apara espaços/pontos nas
// bordas (Windows não aceita nome terminado em espaço ou ponto). Preserva
// maiúsculas/minúsculas — usado também para montar o nome real do arquivo
// salvo, não só para comparar.
func normalizeFSName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return -1
		}
		return r
	}, name)
	return strings.Trim(name, " .")
}

// matchKey normaliza para COMPARAÇÃO de nomes: normalizeFSName + minúsculas.
// Servidor e disco podem devolver a mesma obra com capitalização diferente
// (renomear no Windows Explorer é case-insensitive; o servidor não sabe
// disso) — sem o fold, "Cap 01.cbz" e "CAP 01.CBZ" pareceriam arquivos
// diferentes e o item nunca sairia da lista de "faltantes". Nunca usar para
// nome de arquivo gravado (perderia a capitalização original).
func matchKey(name string) string {
	return strings.ToLower(normalizeFSName(name))
}

// normKey é matchKey preservando o sufixo "/" que marca diretórios no diff.
func normKey(name string) string {
	if strings.HasSuffix(name, "/") {
		return matchKey(strings.TrimSuffix(name, "/")) + "/"
	}
	return matchKey(name)
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
		libNames[matchKey(item.Name)] = true
		if item.FolderID != "" {
			libFolderIDs[item.FolderID] = true
		}
	}
	localSet := make(map[string]bool, len(localDirs))
	for _, name := range localDirs {
		localSet[matchKey(name)] = true
	}

	var plan syncPlan
	for _, name := range localDirs {
		if !libNames[matchKey(name)] {
			plan.addLocal = append(plan.addLocal, name)
		}
	}
	for _, f := range remoteFolders {
		if !f.IsDirectory || libNames[matchKey(f.Name)] || libFolderIDs[f.ID] || localSet[matchKey(f.Name)] {
			continue
		}
		plan.importRemote = append(plan.importRemote, f)
	}
	return plan
}

// findOrphans lista as obras com pasta local registrada que não existe mais no
// disco E não existe no servidor (por ID ou por nome). Obra sem pasta local mas
// viva no servidor é o estado normal de uma importada, não é órfã.
// exists deve devolver true quando o caminho existe OU quando não dá para saber
// (ex.: erro de permissão) — só "não existe" confirmado conta como ausente.
// ponytail: obra vinculada a pasta fora da pasta padrão pode ser falso positivo;
// o confirm lista os nomes e a tela da obra trata o 404 real.
func findOrphans(library []LibraryItem, serverItems []fsItem, exists func(path string) bool) []string {
	remoteIDs := make(map[string]bool, len(serverItems))
	remoteNames := make(map[string]bool, len(serverItems))
	for _, it := range serverItems {
		if it.IsDirectory {
			remoteIDs[it.ID] = true
			remoteNames[matchKey(it.Name)] = true
		}
	}
	var orphans []string
	for _, item := range library {
		if item.LocalPath == "" || exists(item.LocalPath) {
			continue
		}
		if remoteIDs[item.FolderID] || remoteNames[matchKey(item.Name)] {
			continue
		}
		orphans = append(orphans, item.Name)
	}
	return orphans
}

// existingFolderID acha, entre os itens do servidor, uma pasta com o mesmo nome
// (comparação tolerante a maiúsculas/caracteres de controle) para vincular em
// vez de criar uma duplicada.
func existingFolderID(serverItems []fsItem, name string) (string, bool) {
	key := matchKey(name)
	id, found := "", false
	for _, it := range serverItems {
		if it.IsDirectory && matchKey(it.Name) == key {
			id, found = it.ID, true // com nomes duplicados no servidor, a última vence (comportamento original)
		}
	}
	return id, found
}

// importedItem monta a obra de uma pasta que só existe no servidor. O caminho
// local usa o nome seguro para o disco; a pasta local NÃO é criada.
func importedItem(f fsItem, basePath string) LibraryItem {
	return LibraryItem{Name: f.Name, LocalPath: filepath.Join(basePath, normalizeFSName(f.Name)), FolderID: f.ID}
}
