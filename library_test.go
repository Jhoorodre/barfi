package main

import (
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
	} {
		if got := normalizeFSName(in); got != want {
			t.Errorf("normalizeFSName(%q) = %q, want %q", in, got, want)
		}
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
