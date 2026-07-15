package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// downloadHost é o gateway de download do buzzheavier (GET /d/{id} com
// Bearer token). O file manager do site usa este host para todos os
// arquivos da conta.
// ponytail: host fixo; se aparecer conta em nó diferente, vira campo de config.
var downloadHost = "https://ts.buzzheavier.com"

// downloadFile baixa o arquivo fileID para destPath, streaming para um
// .part que só vira o arquivo final se o download completar.
// O gateway responde 302 para a página pública quando não autorizado —
// redirects não são seguidos e viram erro.
func downloadFile(token, fileID, destPath string) error {
	req, err := http.NewRequest(http.MethodGet, downloadHost+"/d/"+fileID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("servidor retornou %d (sem permissão ou arquivo indisponível)", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.Create(destPath + ".part")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), destPath)
}
