# Análise de Tecnologias e Padrões — Barfi

## Contexto

Barfi é um cliente CLI para upload de arquivos em multipart com suporte a paralelismo, configuração multi-perfil, modo interativo TUI e progresso adaptável. Este documento lista tecnologias, features e padrões reutilizáveis.

---

## 1. Dependências (go.mod)

### 1.1 Charmbracelet Suite (TUI framework)

| Pacote | Versão | Função |
|--------|--------|--------|
| `charmbracelet/huh` | v1.0.0 | Formulários e menus interativos |
| `charmbracelet/bubbletea` | v1.3.6 | Framework base para TUI |
| `charmbracelet/bubbles` | v0.21.1 | Componentes reutilizáveis (inputs, spinners) |
| `charmbracelet/lipgloss` | v1.1.0 | Styling e cores |
| `charmbracelet/x/term` | v0.2.1 | Detecção de capacidades do terminal |
| `charmbracelet/x/ansi` | v0.9.3 | Processamento de códigos ANSI |
| `charmbracelet/colorprofile` | v0.2.3 | Detecção de perfil de cores |
| `charmbracelet/x/cellbuf` | v0.0.13 | Buffer de células de terminal |
| `charmbracelet/x/exp/strings` | — | Funções experimentais de strings |

### 1.2 Rendering e styling

| Pacote | Versão | Função |
|--------|--------|--------|
| `catppuccin/go` | v0.3.0 | Tema Catppuccin |
| `lucasb-eyer/go-colorful` | v1.2.0 | Manipulação de cores |
| `aymanbagabas/go-osc52/v2` | v2.0.1 | Protocolo OSC 52 (clipboard) |
| `muesli/termenv` | v0.16.0 | Terminal environment com capabilities |
| `muesli/ansi` | — | Sequências ANSI |

### 1.3 Utilitários de terminal

| Pacote | Versão | Função |
|--------|--------|--------|
| `mattn/go-isatty` | v0.0.20 | Detecta se é TTY |
| `mattn/go-runewidth` | v0.0.16 | Largura de caracteres Unicode |
| `mattn/go-localereader` | v0.0.1 | Leitura respeitando locale |
| `erikgeiser/coninput` | — | Captura de entrada em modo raw |
| `muesli/cancelreader` | v0.2.2 | Leitura com suporte a cancelamento |
| `rivo/uniseg` | v0.4.7 | Segmentação Unicode |
| `xo/terminfo` | — | Database de informações de terminal |

### 1.4 Utilitários de sistema

| Pacote | Versão | Função |
|--------|--------|--------|
| `atotto/clipboard` | v0.1.4 | Acesso ao clipboard |
| `dustin/go-humanize` | v1.0.1 | Formatação humanizada de tamanhos |
| `mitchellh/hashstructure/v2` | v2.0.2 | Hash de structs Go |

### 1.5 Stdlib estendida

| Pacote | Versão | Função |
|--------|--------|--------|
| `golang.org/x/sync` | v0.15.0 | Primitivas avançadas de sincronização |
| `golang.org/x/sys` | v0.33.0 | Chamadas de sistema específicas de SO |
| `golang.org/x/text` | v0.23.0 | Encoding e texto Unicode |

---

## 2. Features e funcionalidades

### 2.1 CLI

- Parser de flags com aliases curtos/longos (`-j`/`--workers`, `-q`/`--quiet`, etc.)
- 15 flags, 20 nomes contando aliases — todas documentadas em `--help`
- Argumentos posicionais (arquivos a enviar)
- Output separado: progresso em stderr, link em stdout
- Modo de gerenciamento de configuração (`--config show/set/unset`)
- Detecção de modo (interativo vs batch vs config)

### 2.2 Sistema de configuração multi-perfil

- Múltiplos perfis nomeados com perfil ativo
- Armazenamento JSON em `~/.config/barfi/config.json`
- Permissões seguras (`0600` arquivo, `0700` diretório)
- Precedência: CLI flags > env vars > config file
- Migração automática e silenciosa do formato antigo (flat) para multi-perfil
- Auto-correção: `locationId` preenchido com ID de pasta → movido para `parentId` automaticamente
- Gerenciamento via CLI: `--config set/unset/show`
- Gerenciamento interativo: trocar, editar, criar, deletar perfis
- `defaultNote` por perfil (apenas via modo interativo)

### 2.3 Upload de arquivos

- Upload simples e multipart (split automático em partes)
- Tamanho de parte auto-calculado: padrão 100 MB; arquivos menores usam seu tamanho real (mínimo 5 MB)
- Upload paralelo com workers configuráveis via `-j` (padrão: 5)
- Limites do protocolo BUS: máximo 1 TB por arquivo, máximo 10.000 partes
- Upload recursivo de diretórios (`-r`/`--recursive`)
- Upload via guest link (sem autenticação)
- Upload com nota opcional (máx. 500 chars, base64-encoded na URL)
- Retry automático com backoff exponencial: 5 tentativas por parte, 1→2→4→8→16 s (cap), somente para erros 5xx
- Rollback de progresso entre tentativas (sem double-counting)
- Detecção de sessão expirada (`errExpired`) com mensagem clara
- Batch retry interativo: após lote com falhas, oferece retentar somente os arquivos falhados

### 2.4 Progresso e feedback

- Três modos via factory `newProgress(quiet bool, name string)`:
  - `noopProgress` — quando `--quiet`
  - `barProgress` — quando stderr é TTY (barra animada a 10 Hz com `\r`)
  - `plainProgress` — quando stderr é pipe (uma linha por 10%)
- Barra visual com caracteres `#` (enviado), `:` (em progresso), `.` (pendente) por parte
- Velocidade média calculada sobre janela deslizante de 2 s (ring buffer de 20 amostras)
- ETA dinâmico
- Hard truncate do nome do arquivo para nunca quebrar linha
- Detecção de largura do terminal com fallback
- Tamanhos formatados (KiB, MiB, GiB) via `dustin/go-humanize`

### 2.5 Modo interativo (TUI)

- Loop principal com switch de ação (upload, gerenciar, biblioteca, perfis, sair)
- Gerenciador de perfis (trocar, editar, criar, deletar)
- Gerenciador de pastas no servidor: navegar, criar, renomear, mover, deletar, favoritar
- Edição de notas de arquivos no servidor
- Seleção e exclusão em lote de itens
- Biblioteca: vincular pastas locais a pastas remotas; preview de conteúdo (máx. 15 itens por seção); sincronização com servidor
- Input de caminho com validação de existência
- Auto-detecção: diretório → pergunta sobre modo recursivo
- Formulários dinâmicos com `.Validate()` inline
- Tratamento de `huh.ErrUserAborted` (Ctrl+C volta ao menu, não encerra)
- Persist imediato após cada alteração (`saveAndReloadCfg`)
- Path normalization para WSL2: `C:\path` → `/mnt/c/path` (via `wslpath -u` com fallback manual)

### 2.6 Validações

- Arquivo existe e não está vazio
- Arquivo ≤ 1 TB
- Server URL não vazio
- Token obrigatório se `parentId` fornecido ou se não for guest link
- `parentId` e `guestUploadLinkId` mutuamente exclusivos
- Workers ≥ 1
- Tamanho de parte entre 5–100 MB
- Número total de partes ≤ 10.000
- Nota ≤ 500 caracteres
- Nome de perfil não pode ser vazio ou duplicado
- Proteção contra deletar o único perfil existente

### 2.7 Tratamento de erros

- Tipos de erro customizados: `errExpired`, `errPartTooLarge`, `serverError` (wraps 4xx)
- Exit codes: `0` (sucesso), `1` (falha), `2` (erro de uso CLI), `130` (Ctrl+C)
- Extração de mensagem amigável do corpo de resposta HTTP de erro
- Graceful shutdown via contexto: SIGINT/SIGTERM propagam cancelamento por toda a pilha
- Retry automático com backoff exponencial (5xx); 4xx não são reintentados

### 2.8 Recursos especializados

- Suporte a WSL2 (detecção e conversão de paths Windows)
- Detecção de terminal (TTY vs pipe) para escolha de modo de progresso
- Terminal width detection com build tags por SO (`termwidth_unix.go` / `termwidth_windows.go`)
- JSON output (`--json`): resposta bruta do servidor
- Quiet mode (`--quiet`): somente link em stdout
- `--save`: persiste as configurações resolvidas (flags + env aplicados) no perfil ativo

---

## 3. Padrões de design reutilizáveis

### 3.1 Flag parsing pattern
**Arquivo**: `main.go:53`

```go
type cliOptions struct { /* campos */ }

func parseFlags(args []string) (*cliOptions, error) {
    fs := flag.NewFlagSet("barfi", flag.ContinueOnError)
    fs.StringVar(&opts.token, "token", "", "...")
    fs.StringVar(&opts.token, "t", "", "...") // alias
    // ...
    return opts, fs.Parse(args)
}
```

Destaques: `flag.ContinueOnError` permite testes sem `os.Exit`; aliases via `fs.StringVar` duplo; separação de modo posicional vs modo `--config`.

Reutilizável para: qualquer CLI com múltiplas flags, aliases e precedência.

---

### 3.2 Multi-profile config pattern
**Arquivo**: `config.go` (completo, 108 linhas)

```go
type Config struct { /* campos omitempty */ }
type MultiConfig struct {
    ActiveProfile string            `json:"activeProfile"`
    Profiles      map[string]Config `json:"profiles"`
}
func loadConfig(path string) (MultiConfig, error) { /* migração automática */ }
func saveConfig(path string, mCfg MultiConfig) error { /* 0600, JSON indentado */ }
```

Destaques: migração silenciosa de formato antigo; permissões seguras (`0600`/`0700`); fallback para perfil "Padrão" se config não existe.

Reutilizável para: qualquer aplicação com múltiplos ambientes/contas.

---

### 3.3 Worker pool + job queue pattern
**Arquivo**: `upload.go:279` (`uploadAllParts`)

```go
ctx, cancel := context.WithCancel(parentCtx)
defer cancel()
jobs := make(chan int, totalParts)
var wg sync.WaitGroup
var firstErr atomic.Pointer[error]

for w := 0; w < workers; w++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for partNum := range jobs {
            if ctx.Err() != nil { return }
            if err := u.uploadPart(ctx, partNum); err != nil {
                firstErr.CompareAndSwap(nil, &err)
                cancel()
                return
            }
        }
    }()
}
for i := 1; i <= totalParts; i++ { jobs <- i }
close(jobs)
wg.Wait()
```

Destaques: `atomic.Pointer` para capturar primeiro erro sem mutex; cancelamento propaga imediatamente a todos os workers.

Reutilizável para: upload paralelo, processamento em lote, qualquer trabalho concorrente.

---

### 3.4 Signal handling pattern (graceful shutdown)
**Arquivo**: `main.go:221`

```go
ctx, stop := signal.NotifyContext(context.Background(),
    os.Interrupt, syscall.SIGTERM)
defer stop()
// ctx é passado para toda a pilha de upload
if errors.Is(err, context.Canceled) {
    return 130
}
```

Reutilizável para: qualquer CLI que precise de Ctrl+C limpo.

---

### 3.5 E2E testing pattern
**Arquivo**: `main_test.go`

Estrutura:
- Compila o binário em `t.TempDir()`
- Sobe fake HTTP server em-processo (`httptest.NewServer`)
- Isola config via `HOME` e `XDG_CONFIG_HOME` em temp dirs
- Captura estado do fake server em struct com callbacks
- Substitui `sleepBackoff` por no-op para testes rápidos

Reutilizável para: qualquer CLI que precise de testes end-to-end confiáveis sem mocks frágeis.

---

### 3.6 Interactive CLI with sub-menus pattern
**Arquivo**: `interactive.go` (1639 linhas)

```go
import "github.com/charmbracelet/huh"

for {
    var action string
    if err := huh.NewForm(huh.NewGroup(
        huh.NewSelect[string]().
            Title("Menu").
            Options(huh.NewOption("Opção", "opt")).
            Value(&action),
    )).Run(); err != nil {
        return err // ErrUserAborted = Ctrl+C
    }
    switch action {
    case "back": return nil
    case "opt": // ...
    }
}
```

Destaques: loop infinito com switch; `huh.ErrUserAborted` para voltar; validação inline via `.Validate()`; persist imediato após mudança.

---

### 3.7 Adaptive progress UI pattern
**Arquivo**: `progress.go` (473 linhas)

```go
type Progress interface {
    Start(total int64, tracker *partTracker)
    Finish(success bool)
}

// progress.go:465
func newProgress(quiet bool, name string) Progress {
    if quiet { return noopProgress{} }
    if isTerminal(os.Stderr) { return newBarProgress(os.Stderr, name) }
    return newPlainProgress(os.Stderr, name)
}
```

Destaques: factory escolhe implementação baseada em TTY; `barProgress` roda goroutine de render a 10 Hz com `\r`; `plainProgress` imprime linha por 10%; ambos usam `atomic.Int64` sem locks; velocidade calculada via ring buffer de 2 s.

---

## 4. Estrutura de arquivos

```
barfi/
├── main.go              # Entry point, flag parsing, orchestração (602 linhas)
├── upload.go            # Upload multipart, HTTP, retry, worker pool (411 linhas)
├── config.go            # Config multi-perfil, migração, persistência (108 linhas)
├── interactive.go       # Modo interativo, TUI, menus, biblioteca (1639 linhas)
├── progress.go          # Progress tracking, rendering TTY/pipe (473 linhas)
├── partsize.go          # Constantes do protocolo BUS, cálculo de parte (83 linhas)
├── termwidth_unix.go    # Terminal width detection — Unix
├── termwidth_windows.go # Terminal width detection — Windows
├── main_test.go         # Testes E2E, fake BUS server
├── upload_test.go       # Testes unitários — upload
├── config_test.go       # Testes unitários — config
├── progress_test.go     # Testes unitários — progress
├── partsize_test.go     # Testes unitários — partsize
├── go.mod               # Module definition
├── go.sum               # Hashes de dependências
├── CLAUDE.md            # Guia de desenvolvimento
├── README.md            # Documentação em inglês (padrão GitHub)
├── README.pt-BR.md      # Documentação em português
├── README               # Documentação em português (legado, sem extensão)
├── barfi.bat            # Wrapper batch para Windows
└── docs/
    ├── buzzheavier-api.md          # Referência da API buzzheavier.com
    └── TECHNOLOGIES_AND_PATTERNS.md # Este arquivo
```

---

## 5. Protocolo BUS (partsize.go)

Constantes espelhadas de `bus/protocol.go` no servidor — alterar sem coordenação com o servidor quebra uploads:

| Constante | Valor | Descrição |
|-----------|-------|-----------|
| `MinPartSize` | 5 MB | Tamanho mínimo de parte |
| `MaxPartSize` | 100 MB | Tamanho máximo / padrão de parte |
| `MaxParts` | 10.000 | Máximo de partes por upload |
| `MaxFileSize` | 1 TB | Tamanho máximo de arquivo |
| `maxRetries` | 5 | Tentativas por parte (5xx) |

Headers HTTP usados no upload multipart: `Upload-Length`, `Upload-Part-Number`.

---

## 6. Comandos úteis

```bash
# Build de produção
CGO_ENABLED=0 go build -ldflags="-s -w" -o barfi ./

# Todos os testes
go test ./...

# Teste específico
go test -run TestCLI_EndToEnd ./...

# Verbose
go test -v ./...

# Modo interativo
./barfi

# Config
./barfi --config show
./barfi --config set server https://buzzheavier.com
./barfi --config set workers 10
```

Flags de build:
- `CGO_ENABLED=0` — sem dependências C (portabilidade)
- `-s -w` — remove debug symbols e DWARF (binário menor)

Go version mínima: **1.23.0**
