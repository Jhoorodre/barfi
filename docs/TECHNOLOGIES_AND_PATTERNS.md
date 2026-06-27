# Análise de Tecnologias e Features - Barfi

## Contexto
O projeto Barfi é um cliente CLI robusto para upload de arquivos em multipart com suporte a paralelismo, configuração multi-perfil, modo interativo, e progresso adaptável. Este documento lista todas as tecnologias, features, funcionalidades e padrões reutilizáveis para aplicar em um novo projeto.

---

## 1. TECNOLOGIAS E DEPENDÊNCIAS (24 principais)

### 1.1 Charmbracelet Suite (Framework TUI)
Framework completo para construir interfaces de terminal interativas:
- **charmbracelet/huh** (v1.0.0) - Formulários e questões interativas
- **charmbracelet/bubbletea** (v1.3.6) - Framework base para TUI
- **charmbracelet/bubbles** (v0.21.1) - Componentes reutilizáveis (inputs, spinners)
- **charmbracelet/lipgloss** (v1.1.0) - Styling e renderização com cores
- **charmbracelet/x/term** (v0.2.1) - Detecção de capacidades do terminal
- **charmbracelet/x/ansi** (v0.9.3) - Processamento de códigos ANSI
- **charmbracelet/colorprofile** (v0.2.3) - Detecção de profiles de cores
- **charmbracelet/x/cellbuf** (v0.0.13) - Buffer eficiente de células
- **charmbracelet/x/exp/strings** - Funções experimentais de strings

### 1.2 Renderização e Styling
- **catppuccin/go** (v0.3.0) - Tema Catppuccin para terminal
- **lucasb-eyer/go-colorful** (v1.2.0) - Manipulação de cores
- **aymanbagabas/go-osc52** (v2.0.1) - Protocolo OSC 52 (clipboard)
- **muesli/termenv** (v0.16.0) - Ambiente de terminal com capabilities
- **muesli/ansi** - Processamento de sequências ANSI

### 1.3 Utilidades de Terminal
- **mattn/go-isatty** (v0.0.20) - Detecta se é TTY
- **mattn/go-runewidth** (v0.0.16) - Largura de caracteres Unicode
- **mattn/go-localereader** (v0.0.1) - Leitura respeitando locale
- **erikgeiser/coninput** - Captura de entrada em modo raw
- **muesli/cancelreader** (v0.2.2) - Leitura com suporte a cancelamento
- **rivo/uniseg** (v0.4.7) - Segmentação Unicode
- **xo/terminfo** - Database de informações de terminal

### 1.4 Utilidades de Sistema
- **atotto/clipboard** (v0.1.4) - Acesso ao clipboard do sistema
- **dustin/go-humanize** (v1.0.1) - Formatação humanizada de números
- **mitchellh/hashstructure** (v2.0.2) - Hash de estruturas Go

### 1.5 Libs Padrão Golang
- **golang.org/x/sync** (v0.15.0) - Primitivas avançadas de sincronização
- **golang.org/x/sys** (v0.33.0) - Chamadas de sistema específicas
- **golang.org/x/text** (v0.23.0) - Suporte a encoding e texto Unicode

---

## 2. FEATURES E FUNCIONALIDADES PRINCIPAIS

### 2.1 CLI (Command-Line Interface)
✓ Parser de flags com alias curtos/longos  
✓ 30+ flags com descrições em português  
✓ Argumentos posicionais (arquivos)  
✓ Output separado em stdout/stderr  
✓ Modo de gerenciamento de configuração (--config show/set/unset)  
✓ Menu de ajuda contextual com exemplos  
✓ Detecção de modo (interativo vs batch vs config)  

### 2.2 Sistema de Configuração Multi-Perfil
✓ Múltiplos perfis nomeados com ativação  
✓ Armazenamento JSON em ~/.config/barfi/config.json  
✓ Permissões seguras (0o600 para arquivo, 0o700 para dir)  
✓ Precedência: CLI flags > env vars > config file  
✓ Migração automática e silenciosa de formato antigo para novo  
✓ Auto-correção de erros de user (ex: ParentId em LocationId)  
✓ Gerenciamento via CLI: --config set/unset/show  
✓ Gerenciamento interativo: trocar perfil, editar, criar, deletar  

### 2.3 Upload de Arquivos
✓ Upload simples de arquivo único  
✓ Upload em multipart (split em partes)  
✓ Upload paralelo (5-10000 workers configuráveis)  
✓ Upload recursivo de diretórios (-r/--recursive)  
✓ Upload via guest link (sem autenticação)  
✓ Upload com nota opcional (≤500 chars)  
✓ Suporte a tamanho de parte customizado (5-100MB)  
✓ Retry automático com backoff exponencial (5 tentativas)  
✓ Detecção de session expirada com mensagem clara  
✓ Rollback de progresso em retry (sem double-counting)  

### 2.4 Progresso e Feedback
✓ Três modos de progresso: silent (--quiet), TTY (bar), pipe (texto)  
✓ Barra visual com caracteres (#, :, .) por parte  
✓ Cálculo de velocidade média sobre últimos 2s  
✓ ETA dinâmico (tempo estimado restante)  
✓ Detecção de redimensionamento de terminal  
✓ Suporte a ANSI escape sequences (\r, \x1b[J, \x1b[A)  
✓ Hard truncate para nunca quebrar linha  
✓ Output formatado com tamanhos humanizados (KiB, MiB, GiB)  

### 2.5 Modo Interativo (TUI)
✓ Menu principal com opções de upload, perfis, sair  
✓ Gerenciador de perfis (trocar, editar, criar, deletar)  
✓ Input de caminho com validação de existência  
✓ Auto-detecção de diretório vs arquivo  
✓ Pergunta sobre upload recursivo se for diretório  
✓ Campo de nota com sugestão de padrão do perfil  
✓ Formulários dinâmicos (campos adicionados conforme necessário)  
✓ Validação inline com mensagens de erro  
✓ Path normalization para WSL (C:\path → /mnt/c/path)  
✓ Suporte a retry interativo para uploads falhados  

### 2.6 Validações
✓ Arquivo existe e não está vazio  
✓ Arquivo ≤ 1TB  
✓ Server URL não vazio  
✓ Token obrigatório se ParentId ou não guest link  
✓ ParentId e GuestUploadLinkId mutuamente exclusivos  
✓ Workers ≥ 1  
✓ Part size entre 5-100MB  
✓ Número total de parts ≤ 10000  
✓ Nota ≤ 500 caracteres  
✓ Perfil não pode ser vazio ou duplicado  
✓ Proteção contra deletar único perfil  

### 2.7 Tratamento de Erros
✓ Custom error types: `errExpired`, `errPartTooLarge`, `serverError`  
✓ Exit codes: 0 (sucesso), 1 (falha), 2 (erro CLI), 130 (Ctrl+C)  
✓ Extração de mensagens amigáveis de respostas de erro  
✓ Graceful cancelamento com Ctrl+C/SIGTERM  
✓ Retry automático com backoff exponencial  
✓ Batch retry interativo para múltiplos arquivos  
✓ Resumo de falhas com opcão de retentativa  

### 2.8 Recursos Especializados
✓ Suporte a WSL (detecção e conversão de paths Windows)  
✓ Detecção de terminal (TTY vs pipe)  
✓ Terminal width detection com fallback  
✓ JSON output mode (--json) com raw server response  
✓ Quiet mode (--quiet) apenas link em stdout  
✓ Versioning (--version)  
✓ Help contextual (--help)  

---

## 3. PADRÕES DE DESIGN REUTILIZÁVEIS

### 3.1 Flag Parsing Pattern
**Arquivos**: main.go:32-91, main.go:95-154

Estrutura:
- Struct `cliOptions` com todos os campos tipados
- `parseFlags()` com `flag.FlagSet` e `flag.ContinueOnError`
- Aliases curtos/longos para mesma flag
- Separação entre modo posicional normal e modo especial (--config)
- Precedência: arquivo < env vars < CLI flags

Reutilizável para: Qualquer CLI que precise de múltiplas flags, aliases, e precedência

### 3.2 Multi-Profile Config Pattern
**Arquivo**: config.go (completo)

Estrutura:
- Struct `Config` com campos individual (omitempty)
- Struct `MultiConfig` com active profile e mapa de perfis
- Auto-migração silenciosa de formato antigo
- Permissões seguras (0o600 arquivo, 0o700 dir)
- Fallback inteligente (cria padrão se não existe)
- JSON indentado para legibilidade

Reutilizável para: Qualquer aplicação que precise suportar múltiplos ambientes/perfis

### 3.3 Worker Pool + Job Queue Pattern
**Arquivo**: upload.go:211-322

Estrutura:
- `partTracker` com atomic.Int64 (sem mutex)
- Job queue em channel com buffer
- Pool de workers com WaitGroup
- Primeiro erro cancela contexto
- Retry com exponential backoff
- Rollback de progresso entre tentativas

Reutilizável para: Upload paralelo, processamento batch, any concurrent work

### 3.4 Signal Handling Pattern (Graceful Shutdown)
**Arquivo**: main.go:311-312, 400-407

Estrutura:
- `signal.NotifyContext()` para converter sinais em contexto
- Propagação via contexto através de toda pilha
- Context.Err() checks em pontos críticos
- defer stop() para limpeza
- Exit code 130 para Ctrl+C

Reutilizável para: Qualquer aplicação que precise lidar com Ctrl+C/SIGTERM

### 3.5 E2E Testing Pattern
**Arquivo**: main_test.go, upload_test.go

Estrutura:
- Compile binário em temp dir
- Fake HTTP server em-processo (httptest.Server)
- Isolamento de config (HOME e XDG_CONFIG_HOME em tempdir)
- State capture em struct do fake server
- Knobs testáveis via callbacks
- Stubs de timing (sleepBackoff) para testes rápidos

Reutilizável para: Qualquer CLI que precise de teste end-to-end confiável

### 3.6 Interactive CLI with Sub-Menus Pattern
**Arquivo**: interactive.go (completo)

Estrutura:
- Loop com switch de ação (fácil adicionar opções)
- Formulários dinâmicos construídos conforme necessário
- Validação inline com `.Validate()`
- Formulários aninhados (sub-menus)
- Defaults sensatos (pré-preenchidos)
- Tratamento de `ErrUserAborted` (permite voltar)
- Path normalization (WSL support)
- Persist imediato após alteração

Reutilizável para: CLI interativa com menus complexos

### 3.7 Adaptive Progress UI Pattern
**Arquivo**: progress.go (completo)

Estrutura:
- Interface abstrata `Progress`
- Factory `newProgress()` que escolhe implementação
- Três implementações: `noopProgress`, `plainProgress`, `barProgress`
- Background goroutine com ticker
- Lock-free com atomic.Int64
- Speed window (ringbuffer de 2s)
- Terminal width detection com fallback
- ANSI escape sequences para refresh
- Build tags para code específico de SO

Reutilizável para: Progresso adaptável para qualquer aplicação

---

## 4. ESTRUTURA DE ARQUIVO E ORGANIZAÇÃO

```
barfi/
├── main.go              # CLI entry point, flag parsing, orchestração
├── upload.go            # Core upload logic, HTTP, multipart
├── config.go            # Config management, profiles, persistence
├── interactive.go       # Interactive mode, TUI menus, forms
├── progress.go          # Progress tracking, rendering (TTY/pipe)
├── partsize.go          # Part size calculation, human size parsing
├── termwidth_unix.go    # Terminal width detection (Unix)
├── termwidth_windows.go # Terminal width detection (Windows)
├── main_test.go         # E2E tests, fake BUS server
├── upload_test.go       # Unit tests para upload
├── config_test.go       # Tests para config management
├── progress_test.go     # Tests para progress tracking
├── partsize_test.go     # Tests para part size calculation
├── go.mod               # Module definition
├── go.sum               # Dependencies hash
├── CLAUDE.md            # Development guide
├── README               # User-facing documentation
└── barfi.bat            # Windows batch wrapper (opcional)
```

---

## 5. CHECKLIST DE TECNOLOGIAS/FEATURES PARA NOVO PROJETO

### Essencial (Core)
- [ ] CLI com flags e parsing robusta
- [ ] HTTP client com retry
- [ ] Tratamento de sinais (Ctrl+C)
- [ ] Config multi-ambiente
- [ ] Logging/Output estruturado

### Recomendado (Para qualquer CLI)
- [ ] Flag parsing com aliases
- [ ] Precedência: CLI > env > config file
- [ ] Modo interativo com Charmbracelet/huh
- [ ] Progresso adaptável (TTY vs pipe)
- [ ] E2E tests com fake server

### Avançado (Se relevante)
- [ ] Worker pool para paralelismo
- [ ] Retry com backoff exponencial
- [ ] Multi-perfil/environment config
- [ ] WSL path normalization
- [ ] Terminal width detection
- [ ] ETA cálculo
- [ ] JSON output mode

### Validações Importantes
- [ ] Input validation (tamanho, formato)
- [ ] Mutual exclusion rules
- [ ] Proteção contra user error
- [ ] Mensagens de erro user-friendly
- [ ] Exit codes bem definidos

---

## 6. COMANDO DE BUILD

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o barfi ./
```

Flags explicados:
- `CGO_ENABLED=0`: Desativa C bindings (sem dependências externas)
- `-s -w`: Remove debug symbols e DWARF (binário menor)

Go version necessária: 1.23.0+

---

## 7. PADRÕES PRONTOS PARA COPIAR-COLAR

### Padrão 1: Flag Parser
**De**: main.go:53-91
```go
type cliOptions struct { /* campos */ }
func parseFlags(args []string) (*cliOptions, error) { /* implementation */ }
```

### Padrão 2: Config Multi-Perfil
**De**: config.go (completo)
```go
type Config struct { }
type MultiConfig struct {
    ActiveProfile string
    Profiles map[string]Config
}
func loadConfig(path string) (MultiConfig, error) { }
func saveConfig(path string, mCfg MultiConfig) error { }
```

### Padrão 3: Worker Pool
**De**: upload.go:279-322
```go
ctx, cancel := context.WithCancel(parentCtx)
jobs := make(chan int, totalTasks)
var wg sync.WaitGroup
for w := 0; w < workers; w++ {
    wg.Add(1)
    go func() {
        for job := range jobs {
            if ctx.Err() != nil { return }
            if err := process(job) {
                cancel()
                return
            }
        }
        wg.Done()
    }()
}
wg.Wait()
```

### Padrão 4: Signal Handling
**De**: main.go:311-312
```go
ctx, stop := signal.NotifyContext(context.Background(), 
    os.Interrupt, syscall.SIGTERM)
defer stop()
// Passa ctx adiante...
if errors.Is(err, context.Canceled) {
    os.Exit(130)
}
```

### Padrão 5: Interactive Menu
**De**: interactive.go (completo)
```go
import "github.com/charmbracelet/huh"

form := huh.NewForm(
    huh.NewGroup(
        huh.NewSelect[string]().
            Title("Menu").
            Options(huh.NewOption("Option", "opt")).
            Value(&selected),
    ),
)
if err := form.Run(); err != nil {
    return err
}
```

### Padrão 6: Progresso Adaptável
**De**: progress.go (completo)
```go
type Progress interface {
    Start(total int64, tracker *ProgressTracker)
    Finish(success bool)
}

func newProgress(quiet bool) Progress {
    if quiet { return noopProgress{} }
    if isTerminal(os.Stderr) { return newBarProgress() }
    return newPlainProgress()
}
```

---

## 8. COMANDOS ÚTEIS

```bash
# Build
CGO_ENABLED=0 go build -ldflags="-s -w" -o barfi ./

# Run all tests
go test ./...

# Run single test
go test -run TestName ./...

# Run with verbose output
go test -v ./...

# Interactive mode
./barfi

# Config management
./barfi --config show
./barfi --config set server https://example.com
./barfi --config set workers 10
```

---

## Resumo Final

O projeto Barfi é um **excelente template** para novas aplicações CLI que precisam de:

1. **Interface robusta**: Flags com aliases, modos múltiplos, TUI interativa
2. **Configuração flexível**: Multi-perfil, migração automática, precedência clara
3. **Operações complexas**: Upload paralelo, retry, progress tracking
4. **Confiabilidade**: Graceful shutdown, signal handling, E2E tests
5. **UX polida**: Progresso adaptável (TTY/pipe), terminal detection, humanização

Todos os **7 padrões principais** são **copy-paste ready** com mínimas adaptações.
