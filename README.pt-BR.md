```
########     ###    ########  ######## ####
##     ##   ## ##   ##     ## ##        ##
##     ##  ##   ##  ##     ## ##        ##
########  ##     ## ########  ######    ##
##     ## ######### ##   ##   ##        ##
##     ## ##     ## ##    ##  ##        ##
########  ##     ## ##     ## ##       ####
```

> Uploader de linha de comando para [buzzheavier.com](https://buzzheavier.com)

[🇺🇸 English](README.md) | [🇧🇷 Português](README.pt-BR.md)

---

### Instalação

```bash
go install github.com/Jhoorodre/barfi@latest
```

Ou compilar da fonte:

```bash
git clone https://github.com/Jhoorodre/barfi
cd barfi && CGO_ENABLED=0 go build -ldflags="-s -w" -o barfi ./
```

Binários pré-compilados estão disponíveis para Linux (amd64, arm64) nas [releases](https://github.com/Jhoorodre/barfi/releases).
macOS e Windows precisam compilar da fonte.

> Requer Go 1.23.0 ou superior.

---

### Configuração inicial

```bash
barfi --config set server https://buzzheavier.com
barfi --config set token SEU_TOKEN
barfi --config set workers 5
barfi --config show
```

---

### Uso básico

```bash
# Enviar um arquivo
barfi arquivo.txt

# Enviar para uma pasta específica
barfi --parent-id=ID_DA_PASTA arquivo.txt

# Enviar vários arquivos de uma vez
barfi foto1.jpg foto2.jpg video.mp4

# Enviar todos os arquivos de um diretório (recursivo)
barfi -r ./minha-pasta/

# Enviar com uma nota (máx. 500 caracteres)
barfi --note="versão final" arquivo.zip

# Enviar via link de convidado (sem token)
barfi --guest-upload-link-id=ID_DO_LINK arquivo.txt

# Saída só com o link, para uso em scripts
LINK=$(barfi -q arquivo.txt)

# Sobrescrever workers e tamanho de parte nesta execução
barfi -j 10 --part-size=25MB arquivo-grande.bin

# Persistir as flags atuais no perfil ativo
barfi --parent-id=ID_DA_PASTA --workers=10 --save
```

---

### Como o upload funciona

Os arquivos são divididos em partes e enviados em paralelo:

- **Tamanho da parte**: calculado automaticamente — padrão de 100 MB; arquivos menores que 100 MB usam o tamanho real, mínimo de 5 MB. Sobrescreva com `--part-size`.
- **Workers**: 5 goroutines por padrão, configurável com `-j`.
- **Retry**: até 5 tentativas por parte em erros 5xx, backoff exponencial (1 → 2 → 4 → 8 → 16 s). Erros 4xx não são reintentados.
- **Limites**: máximo de 1 TB por arquivo; máximo de 10.000 partes por upload.
- **Retry em lote**: após enviar um lote de arquivos, as falhas são oferecidas para retry interativo.
- **Progresso**: barra animada a 10 Hz quando stderr é um TTY; uma linha por 10% quando em pipe; somente o link no stdout com `--quiet`.

---

### Modo interativo

Execute `barfi` sem argumentos para abrir o modo interativo com TUI completa:

```bash
barfi
```

O modo interativo oferece:

- **Enviar arquivos** — seleção de arquivos e pasta de destino via menus; pergunta sobre modo recursivo para diretórios
- **Gerenciar Buzzheavier** — navegue, crie, renomeie, mova e exclua pastas no servidor; gerencie favoritos; edite notas de arquivos; seleção e exclusão em lote
- **Biblioteca** — vincule pastas locais a pastas remotas para envios recorrentes; exibe preview do conteúdo (até 15 itens por seção); sincronize caminhos locais com o servidor
- **Gerenciar Perfis** — crie, edite, alterne e exclua perfis de configuração nomeados (ex: contas pessoal e de trabalho)

Usuários de WSL2: caminhos do Windows (ex: `C:\Users\foo`) são normalizados automaticamente para `/mnt/c/Users/foo`.

---

### Referência de flags

| Flag | Atalho | Descrição |
|------|--------|-----------|
| `--server URL` | | URL base do servidor |
| `--token T` | | Token de autenticação |
| `--location-id ID` | `-l` | ID do bucket de armazenamento (onde os arquivos ficam fisicamente) |
| `--parent-id ID` | `-d` | ID da pasta de destino (onde os arquivos aparecem na sua árvore) |
| `--guest-upload-link-id ID` | | ID do link de convidado (sem token necessário) |
| `--note TEXTO` | | Nota do upload (máx. 500 caracteres) |
| `--part-size BYTES` | | Tamanho de cada parte (ex: `25MB`; intervalo: 5 MB – 100 MB) |
| `--workers N` | `-j` | Goroutines paralelas de upload (padrão: 5) |
| `--recursive` | `-r` | Enviar diretórios recursivamente |
| `--quiet` | `-q` | Silencia o progresso; imprime somente o link no stdout |
| `--json` | | Imprime a resposta bruta do servidor como JSON |
| `--save` | | Persiste as configurações resolvidas no perfil ativo |
| `--config ACTION` | | Gerencia a config: `show`, `set CHAVE VALOR`, `unset CHAVE` |
| `--version` | | Exibe a versão |
| `--help` | `-h` | Exibe a ajuda |

Variáveis de ambiente (menor precedência, abaixo do arquivo de config): `BARFI_SERVER`, `BARFI_TOKEN`, `BARFI_LOCATION_ID`.

Precedência: `flags > variáveis de ambiente > arquivo de config`.

---

### Configuração

Armazenada em `~/.config/barfi/config.json` (modo `0600`).

```bash
barfi --config show
barfi --config set server https://buzzheavier.com
barfi --config set token SEU_TOKEN
barfi --config set workers 10
barfi --config set parentId ID_DA_PASTA        # pasta de destino padrão
barfi --config set locationId ID_DO_BUCKET     # bucket de armazenamento padrão
barfi --config unset token
```

Chaves válidas para `--config set/unset`: `server`, `token`, `locationId`, `parentId`, `workers`.

**Múltiplos perfis** são gerenciados pelo modo interativo (menu "Gerenciar Perfis"). Cada perfil armazena seu próprio `server`, `token`, `parentId`, `locationId`, `workers` e `defaultNote`. Perfis são úteis para separar contas pessoal e de trabalho ou servidores diferentes.

Configs antigas no formato plano (pré-0.1.0) são migradas automaticamente para o formato de perfis no primeiro carregamento.

---

### Códigos de saída

| Código | Significado |
|--------|-------------|
| `0` | Sucesso |
| `1` | Falha no upload |
| `2` | Erro de uso |
| `130` | Interrompido (Ctrl+C) |

---

### Referência da API

[docs/buzzheavier-api.md](docs/buzzheavier-api.md) — referência completa da API buzzheavier.com (endpoints, parâmetros, exemplos curl, discrepâncias conhecidas).
