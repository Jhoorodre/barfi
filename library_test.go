package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiffNames(t *testing.T) {
	cases := []struct {
		name          string
		local, remote []string
		want          libDiff
	}{
		{
			name:   "sincronizado",
			local:  []string{"cap-01", "cap-02"},
			remote: []string{"cap-01", "cap-02"},
			want:   libDiff{both: []string{"cap-01", "cap-02"}},
		},
		{
			name:   "falta enviar e falta baixar",
			local:  []string{"cap-01", "cap-03"},
			remote: []string{"cap-01", "cap-02"},
			want: libDiff{
				both:       []string{"cap-01"},
				localOnly:  []string{"cap-03"},
				remoteOnly: []string{"cap-02"},
			},
		},
		{
			name:   "remoto vazio",
			local:  []string{"a"},
			remote: nil,
			want:   libDiff{localOnly: []string{"a"}},
		},
		{
			name:   "local vazio",
			local:  nil,
			remote: []string{"b"},
			want:   libDiff{remoteOnly: []string{"b"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := diffNames(tc.local, tc.remote)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("diffNames(%v, %v) = %+v, want %+v", tc.local, tc.remote, got, tc.want)
			}
		})
	}
}

func TestNormalizeFSName(t *testing.T) {
	for in, want := range map[string]string{
		"Tower of God: Parte 2 – O Retorno do Príncipe": "Tower of God Parte 2 – O Retorno do Príncipe",
		"cap-01.cbz":        "cap-01.cbz",
		`a<b>c:d"e|f?g*`:    "abcdefg",
		" nome com borda. ": "nome com borda",
		"..":                "",
		"cap\x0101\x7f.cbz": "cap01.cbz", // caracteres de controle (0x01, DEL) removidos
	} {
		if got := normalizeFSName(in); got != want {
			t.Errorf("normalizeFSName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchKey(t *testing.T) {
	cases := [][2]string{
		{"CAP 01.CBZ", "cap 01.cbz"},
		{"Tower of God: X", "tower of god x"}, // ':' removido + minúsculas juntos
	}
	for _, c := range cases {
		if matchKey(c[0]) != matchKey(c[1]) {
			t.Errorf("matchKey(%q) != matchKey(%q): %q vs %q", c[0], c[1], matchKey(c[0]), matchKey(c[1]))
		}
	}
	// normalizeFSName preserva capitalização — matchKey não deve alterar o disco.
	if normalizeFSName("CAP 01.cbz") != "CAP 01.cbz" {
		t.Error("normalizeFSName não deve alterar capitalização")
	}
}

func TestDiffNamesNormalizaWindows(t *testing.T) {
	d := diffNames(
		[]string{"Cap 01.cbz", "EXTRA/"},
		[]string{"Cap: 01.cbz", "EXTRA/", "Cap: 02.cbz"},
	)
	if len(d.both) != 2 || len(d.localOnly) != 0 {
		t.Errorf("both = %v, localOnly = %v; esperava casar ignorando ':'", d.both, d.localOnly)
	}
	if len(d.remoteOnly) != 1 || d.remoteOnly[0] != "Cap: 02.cbz" {
		t.Errorf("remoteOnly = %v", d.remoteOnly)
	}
}

func TestDiffNamesCaseInsensitive(t *testing.T) {
	d := diffNames([]string{"Cap 01.cbz"}, []string{"CAP 01.CBZ"})
	if len(d.both) != 1 || len(d.localOnly) != 0 || len(d.remoteOnly) != 0 {
		t.Errorf("esperava casar ignorando maiúsculas/minúsculas: %+v", d)
	}
}

func TestBuildSyncPlanNormalizaWindows(t *testing.T) {
	// Pasta local sem ":" (proibido no Windows) deve casar com a remota com ":".
	localDirs := []string{"Tower of God Parte 2"}
	remoteFolders := []fsItem{{ID: "id2", Name: "Tower of God: Parte 2", IsDirectory: true}}
	plan := buildSyncPlan(nil, localDirs, remoteFolders)
	if len(plan.importRemote) != 0 {
		t.Errorf("importRemote = %+v; pasta remota já coberta pela local homônima", plan.importRemote)
	}
	if len(plan.addLocal) != 1 {
		t.Errorf("addLocal = %v; a pasta local deve entrar (e vincular por nome normalizado)", plan.addLocal)
	}
}

func TestSafeRemoteName(t *testing.T) {
	for name, want := range map[string]bool{
		"cap-01.cbz":       true,
		"Cap 001 - A.cbz":  true,
		"":                 false,
		".":                false,
		"..":               false,
		"../../etc/passwd": false,
		`..\..\x.exe`:      false,
		"sub/arquivo.cbz":  false,
	} {
		if got := safeRemoteName(name); got != want {
			t.Errorf("safeRemoteName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestBuildSyncPlan(t *testing.T) {
	library := []LibraryItem{
		{Name: "ObraA", FolderID: "idA"},
		{Name: "ObraRenomeada", FolderID: "idB"},
	}
	localDirs := []string{"ObraA", "ObraNova"}
	remoteFolders := []fsItem{
		{ID: "idA", Name: "ObraA", IsDirectory: true},                // já na biblioteca (nome)
		{ID: "idB", Name: "NomeAntigoNoServidor", IsDirectory: true}, // já vinculada (FolderID)
		{ID: "idC", Name: "ObraSoNoServidor", IsDirectory: true},     // deve ser importada
		{ID: "idD", Name: "ObraNova", IsDirectory: true},             // homônima de pasta local nova → via addLocal
		{ID: "idE", Name: "arquivo.txt", IsDirectory: false},         // não é pasta
	}

	plan := buildSyncPlan(library, localDirs, remoteFolders)

	wantAdd := []string{"ObraNova"}
	if !reflect.DeepEqual(plan.addLocal, wantAdd) {
		t.Errorf("addLocal = %v, want %v", plan.addLocal, wantAdd)
	}
	if len(plan.importRemote) != 1 || plan.importRemote[0].ID != "idC" {
		t.Errorf("importRemote = %+v, want só idC", plan.importRemote)
	}
}

func TestFindOrphans(t *testing.T) {
	server := []fsItem{
		{ID: "d1", Name: "No Servidor", IsDirectory: true},
		{ID: "f9", Name: "arquivo.txt", IsDirectory: false},
	}
	lib := []LibraryItem{
		{Name: "Só Importada", FolderID: "x"},                            // sem LocalPath: nunca órfã
		{Name: "No Disco", LocalPath: "/base/No Disco", FolderID: "zzz"}, // existe no disco
		{Name: "Órfã", LocalPath: "/base/Órfã", FolderID: "gone"},        // sumiu dos dois lados
		{Name: "Viva por ID", LocalPath: "/base/a", FolderID: "d1"},      // sumiu do disco, existe por ID
		{Name: "NO SERVIDOR", LocalPath: "/base/b", FolderID: "outro"},   // sumiu do disco, existe por nome (case)
		{Name: "arquivo.txt", LocalPath: "/base/c", FolderID: "f9"},      // arquivo no servidor não conta como pasta
	}
	onDisk := map[string]bool{"/base/No Disco": true}
	got := findOrphans(lib, server, func(p string) bool { return onDisk[p] })
	want := []string{"Órfã", "arquivo.txt"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("findOrphans = %v, want %v", got, want)
	}
}

// Erro de permissão no stat (exists=true) nunca pode marcar a obra como órfã.
func TestFindOrphans_UnknownStateIsNotOrphan(t *testing.T) {
	lib := []LibraryItem{{Name: "X", LocalPath: "/sem/permissao", FolderID: "a"}}
	if got := findOrphans(lib, nil, func(string) bool { return true }); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestExistingFolderID(t *testing.T) {
	items := []fsItem{
		{ID: "1", Name: "Berserk", IsDirectory: true},
		{ID: "2", Name: "Berserk", IsDirectory: false}, // arquivo homônimo é ignorado
		{ID: "3", Name: "Vagabond", IsDirectory: true},
	}
	if id, ok := existingFolderID(items, "BERSERK"); !ok || id != "1" {
		t.Errorf("case-insensitive: id=%q ok=%v", id, ok)
	}
	if _, ok := existingFolderID(items, "Outra"); ok {
		t.Error("não deveria achar pasta inexistente")
	}
	if _, ok := existingFolderID(nil, "x"); ok {
		t.Error("lista vazia")
	}
}

func TestImportedItem(t *testing.T) {
	got := importedItem(fsItem{ID: "p1", Name: "Obra: Parte 1"}, "/base")
	if got.Name != "Obra: Parte 1" || got.FolderID != "p1" || got.LocalPath != filepath.Join("/base", normalizeFSName("Obra: Parte 1")) {
		t.Errorf("got %+v", got)
	}
}
