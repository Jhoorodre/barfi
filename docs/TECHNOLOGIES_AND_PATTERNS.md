# Technologies and Patterns — Barfi

## Overview

Barfi is a CLI client for multipart file uploads with parallel workers, multi-profile configuration, an interactive TUI, and adaptive progress rendering. This document maps every dependency, feature, and reusable pattern in the codebase.

---

## 1. Dependencies (go.mod)

### 1.1 Charmbracelet Suite (TUI framework)

| Package | Version | Purpose |
|---------|---------|---------|
| `charmbracelet/huh` | v1.0.0 | Interactive forms and menus |
| `charmbracelet/bubbletea` | v1.3.6 | Core TUI framework |
| `charmbracelet/bubbles` | v0.21.1 | Reusable components (inputs, spinners) |
| `charmbracelet/lipgloss` | v1.1.0 | Styling and colors |
| `charmbracelet/x/term` | v0.2.1 | Terminal capability detection |
| `charmbracelet/x/ansi` | v0.9.3 | ANSI escape code processing |
| `charmbracelet/colorprofile` | v0.2.3 | Color profile detection |
| `charmbracelet/x/cellbuf` | v0.0.13 | Efficient terminal cell buffer |
| `charmbracelet/x/exp/strings` | — | Experimental string utilities |

### 1.2 Rendering and styling

| Package | Version | Purpose |
|---------|---------|---------|
| `catppuccin/go` | v0.3.0 | Catppuccin color theme |
| `lucasb-eyer/go-colorful` | v1.2.0 | Color manipulation |
| `aymanbagabas/go-osc52/v2` | v2.0.1 | OSC 52 clipboard protocol |
| `muesli/termenv` | v0.16.0 | Terminal environment and capabilities |
| `muesli/ansi` | — | ANSI sequence processing |

### 1.3 Terminal utilities

| Package | Version | Purpose |
|---------|---------|---------|
| `mattn/go-isatty` | v0.0.20 | TTY detection |
| `mattn/go-runewidth` | v0.0.16 | Unicode character width |
| `mattn/go-localereader` | v0.0.1 | Locale-aware reader |
| `erikgeiser/coninput` | — | Raw-mode input capture |
| `muesli/cancelreader` | v0.2.2 | Cancellable reader |
| `rivo/uniseg` | v0.4.7 | Unicode segmentation |
| `xo/terminfo` | — | Terminal info database |

### 1.4 System utilities

| Package | Version | Purpose |
|---------|---------|---------|
| `atotto/clipboard` | v0.1.4 | System clipboard access |
| `dustin/go-humanize` | v1.0.1 | Human-readable sizes and numbers |
| `mitchellh/hashstructure/v2` | v2.0.2 | Struct hashing |

### 1.5 Extended stdlib

| Package | Version | Purpose |
|---------|---------|---------|
| `golang.org/x/sync` | v0.15.0 | Advanced synchronization primitives |
| `golang.org/x/sys` | v0.33.0 | OS-specific syscalls |
| `golang.org/x/text` | v0.23.0 | Unicode text and encoding |

---

## 2. Features

### 2.1 CLI

- Flag parser with short/long aliases (`-j`/`--workers`, `-q`/`--quiet`, etc.)
- 15 flags, 20 names counting aliases — all documented under `--help`
- Positional arguments for files to upload
- Separated output: progress on stderr, link on stdout
- Config management mode (`--config show/set/unset`)
- Mode detection: interactive vs batch vs config

### 2.2 Multi-profile configuration system

- Multiple named profiles with an active profile pointer
- JSON storage at `~/.config/barfi/config.json`
- Secure permissions (`0600` file, `0700` directory)
- Precedence: CLI flags > env vars > config file
- Silent automatic migration from old flat format to multi-profile
- Auto-fix: `locationId` set to a folder ID → silently moved to `parentId`
- CLI management: `--config set/unset/show`
- Interactive management: switch, edit, create, delete profiles
- `defaultNote` per profile (interactive mode only)

### 2.3 File uploads

- Single-file and multipart uploads (automatic splitting)
- Auto-calculated part size: default 100 MB; files smaller than 100 MB use their actual size, minimum 5 MB
- Parallel upload with configurable workers via `-j` (default: 5)
- BUS protocol limits: max 1 TB per file, max 10,000 parts
- Recursive directory upload (`-r`/`--recursive`)
- Guest link upload (no token required)
- Optional note per upload (max 500 chars, base64-encoded in URL)
- Automatic retry with exponential backoff: 5 attempts per part, 1 → 2 → 4 → 8 → 16 s (cap), 5xx only
- Progress rollback between retry attempts (no double-counting)
- Expired session detection (`errExpired`) with a clear error message
- Interactive batch retry: after a batch with failures, offers to retry only the failed files

### 2.4 Progress and feedback

- Three modes via factory `newProgress(quiet bool, name string)`:
  - `noopProgress` — when `--quiet`
  - `barProgress` — when stderr is a TTY (animated bar at 10 Hz via `\r`)
  - `plainProgress` — when stderr is a pipe (one line per 10%)
- Visual bar using `#` (uploaded), `:` (in-flight), `.` (pending) per part
- Average speed over a 2 s sliding window (ring buffer of 20 samples)
- Dynamic ETA
- Hard truncation of the filename to prevent line wrapping
- Terminal width detection with fallback
- Human-readable sizes (KiB, MiB, GiB) via `dustin/go-humanize`

### 2.5 Interactive mode (TUI)

- Main loop with action switch (upload, manage, library, profiles, quit)
- Profile manager: switch, edit, create, delete
- Remote folder manager: browse, create, rename, move, delete, bookmark
- File note editor
- Batch select and batch delete
- Library: link local folders to remote folders; content preview (up to 15 items per section); sync local paths against the server
- Path input with existence validation
- Auto-detection: directory → asks about recursive mode
- Dynamic forms with inline `.Validate()` callbacks
- `huh.ErrUserAborted` handling: Ctrl+C returns to menu, does not exit
- Immediate persist after every change (`saveAndReloadCfg`)
- WSL2 path normalization: `C:\path` → `/mnt/c/path` (via `wslpath -u` with manual fallback)

### 2.6 Validations

- File exists and is not empty
- File ≤ 1 TB
- Server URL not empty
- Token required if `parentId` is set or upload is not via guest link
- `parentId` and `guestUploadLinkId` are mutually exclusive
- Workers ≥ 1
- Part size between 5–100 MB
- Total parts ≤ 10,000
- Note ≤ 500 characters
- Profile name cannot be empty or duplicate
- Cannot delete the last remaining profile

### 2.7 Error handling

- Custom error types: `errExpired`, `errPartTooLarge`, `serverError` (wraps 4xx)
- Exit codes: `0` (success), `1` (upload failed), `2` (CLI usage error), `130` (Ctrl+C)
- Friendly message extraction from HTTP error response bodies
- Graceful shutdown via context: SIGINT/SIGTERM propagate cancellation through the entire stack
- Automatic retry with exponential backoff for 5xx; 4xx errors are non-retryable

### 2.8 Specialized features

- WSL2 support: Windows path detection and conversion
- Terminal detection (TTY vs pipe) to select the right progress implementation
- Terminal width detection with OS-specific build tags (`termwidth_unix.go` / `termwidth_windows.go`)
- JSON output (`--json`): raw server response
- Quiet mode (`--quiet`): link only on stdout
- `--save`: persists the fully resolved settings (flags + env merged) to the active profile

---

## 3. Reusable design patterns

### 3.1 Flag parsing pattern
**File**: `main.go:53`

```go
type cliOptions struct { /* fields */ }

func parseFlags(args []string) (*cliOptions, error) {
    fs := flag.NewFlagSet("barfi", flag.ContinueOnError)
    fs.StringVar(&opts.token, "token", "", "...")
    fs.StringVar(&opts.token, "t", "", "...") // alias
    // ...
    return opts, fs.Parse(args)
}
```

`flag.ContinueOnError` enables testing without `os.Exit`; aliases via double `fs.StringVar`; clean separation between positional mode and `--config` mode.

Reusable for: any CLI with multiple flags, aliases, and precedence rules.

---

### 3.2 Multi-profile config pattern
**File**: `config.go` (108 lines)

```go
type Config struct { /* fields, all omitempty */ }
type MultiConfig struct {
    ActiveProfile string            `json:"activeProfile"`
    Profiles      map[string]Config `json:"profiles"`
}
func loadConfig(path string) (MultiConfig, error) { /* auto-migration */ }
func saveConfig(path string, mCfg MultiConfig) error { /* 0600, indented JSON */ }
```

Silent migration from old flat format; secure permissions (`0600`/`0700`); falls back to a default "Padrão" profile if config does not exist.

Reusable for: any application that needs multiple environments or accounts.

---

### 3.3 Worker pool + job queue pattern
**File**: `upload.go:279` (`uploadAllParts`)

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

`atomic.Pointer` captures the first error without a mutex; cancellation propagates immediately to all workers.

Reusable for: parallel uploads, batch processing, any concurrent workload.

---

### 3.4 Signal handling pattern (graceful shutdown)
**File**: `main.go:221`

```go
ctx, stop := signal.NotifyContext(context.Background(),
    os.Interrupt, syscall.SIGTERM)
defer stop()
// ctx is threaded through the entire upload stack
if errors.Is(err, context.Canceled) {
    return 130
}
```

Reusable for: any CLI that needs clean Ctrl+C handling.

---

### 3.5 E2E testing pattern
**File**: `main_test.go`

- Compiles the binary into `t.TempDir()`
- Spins up an in-process fake HTTP server (`httptest.NewServer`)
- Isolates config via `HOME` and `XDG_CONFIG_HOME` pointed at temp dirs
- Captures fake server state in a struct with callbacks
- Replaces `sleepBackoff` with a no-op for fast retry tests

Reusable for: any CLI that needs reliable end-to-end tests without brittle mocks.

---

### 3.6 Interactive CLI with sub-menus pattern
**File**: `interactive.go` (1639 lines)

```go
import "github.com/charmbracelet/huh"

for {
    var action string
    if err := huh.NewForm(huh.NewGroup(
        huh.NewSelect[string]().
            Title("Menu").
            Options(huh.NewOption("Option", "opt")).
            Value(&action),
    )).Run(); err != nil {
        return err // huh.ErrUserAborted on Ctrl+C
    }
    switch action {
    case "back": return nil
    case "opt": // ...
    }
}
```

Infinite loop with action switch; `huh.ErrUserAborted` to go back without exiting; inline `.Validate()` callbacks; immediate persist after every change.

---

### 3.7 Adaptive progress UI pattern
**File**: `progress.go` (473 lines)

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

Factory picks the implementation based on TTY detection; `barProgress` runs a render goroutine at 10 Hz using `\r`; `plainProgress` prints one line per 10%; both use `atomic.Int64` without locks; speed calculated via a 2 s ring buffer.

---

## 4. File structure

```
barfi/
├── main.go              # Entry point, flag parsing, orchestration     (602 lines)
├── upload.go            # Multipart upload, HTTP, retry, worker pool   (411 lines)
├── config.go            # Multi-profile config, migration, persistence (108 lines)
├── interactive.go       # Interactive mode, TUI, menus, library        (1639 lines)
├── progress.go          # Progress tracking, TTY/pipe rendering        (473 lines)
├── partsize.go          # BUS protocol constants, part size calc       (83 lines)
├── termwidth_unix.go    # Terminal width detection — Unix
├── termwidth_windows.go # Terminal width detection — Windows
├── main_test.go         # E2E tests, fake BUS server
├── upload_test.go       # Unit tests — upload
├── config_test.go       # Unit tests — config
├── progress_test.go     # Unit tests — progress
├── partsize_test.go     # Unit tests — partsize
├── go.mod               # Module definition
├── go.sum               # Dependency hashes
├── CLAUDE.md            # Development guide
├── README.md            # English documentation (GitHub default)
├── README.pt-BR.md      # Portuguese documentation
├── README               # Portuguese documentation (legacy, no extension)
├── barfi.bat            # Windows batch wrapper
└── docs/
    ├── buzzheavier-api.md           # buzzheavier.com API reference
    └── TECHNOLOGIES_AND_PATTERNS.md # This file
```

---

## 5. BUS protocol constants (partsize.go)

These mirror `bus/protocol.go` on the server — changing them without coordinating with the server will break uploads:

| Constant | Value | Description |
|----------|-------|-------------|
| `MinPartSize` | 5 MB | Minimum part size |
| `MaxPartSize` | 100 MB | Maximum / default part size |
| `MaxParts` | 10,000 | Hard cap on parts per upload |
| `MaxFileSize` | 1 TB | Maximum file size |
| `maxRetries` | 5 | Attempts per part on 5xx errors |

HTTP headers used in multipart upload: `Upload-Length`, `Upload-Part-Number`.

---

## 6. Useful commands

```bash
# Production build
CGO_ENABLED=0 go build -ldflags="-s -w" -o barfi ./

# Run all tests
go test ./...

# Run a specific test
go test -run TestCLI_EndToEnd ./...

# Verbose output
go test -v ./...

# Interactive mode
./barfi

# Config management
./barfi --config show
./barfi --config set server https://buzzheavier.com
./barfi --config set workers 10
```

Build flags:
- `CGO_ENABLED=0` — no C dependencies (portability, no cgo)
- `-s -w` — strip debug symbols and DWARF info (smaller binary)

Minimum Go version: **1.23.0**
