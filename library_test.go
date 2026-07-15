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
