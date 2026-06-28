#!/usr/bin/env bash
set -euo pipefail

# Verifica formatação
echo "→ Verificando formatação..."
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "Arquivos sem formatação (rode go fmt ./...):"
  echo "$unformatted"
  exit 1
fi

# Roda os testes
echo "→ Rodando testes..."
go test ./...

# Build local (Linux amd64)
echo "→ Build local..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o barfi ./

echo "✓ OK — binário gerado: ./barfi"
