```
########     ###    ########  ######## ####
##     ##   ## ##   ##     ## ##        ##
##     ##  ##   ##  ##     ## ##        ##
########  ##     ## ########  ######    ##
##     ## ######### ##   ##   ##        ##
##     ## ##     ## ##    ##  ##        ##
########  ##     ## ##     ## ##       ####
```

> Command-line file uploader for [buzzheavier.com](https://buzzheavier.com)

[🇺🇸 English](README.md) | [🇧🇷 Português](README.pt-BR.md)

---

### Installation

```bash
go install github.com/burritoflakes/barfi@latest
```

Or build from source:

```bash
git clone https://github.com/burritoflakes/barfi
cd barfi && CGO_ENABLED=0 go build -ldflags="-s -w" -o barfi ./
```

Pre-built binaries are available for Linux (amd64, arm64) on the [releases page](https://github.com/burritoflakes/barfi/releases).
macOS and Windows users must build from source.

> Requires Go 1.23.0 or later.

---

### Initial setup

```bash
barfi --config set server https://buzzheavier.com
barfi --config set token YOUR_TOKEN
barfi --config set workers 5
barfi --config show
```

---

### Basic usage

```bash
# Upload a file
barfi file.txt

# Upload to a specific folder
barfi --parent-id=FOLDER_ID file.txt

# Upload multiple files at once
barfi photo1.jpg photo2.jpg video.mp4

# Upload all files in a directory (recursive)
barfi -r ./my-folder/

# Upload with a note (max 500 chars)
barfi --note="final cut" archive.zip

# Upload via guest link (no token required)
barfi --guest-upload-link-id=LINK_ID file.txt

# Link-only output for scripting
LINK=$(barfi -q file.txt)

# Override workers and part size for this run
barfi -j 10 --part-size=25MB large-file.bin

# Persist current flags to the active profile
barfi --parent-id=FOLDER_ID --workers=10 --save
```

---

### How uploads work

Files are split into parts and uploaded in parallel:

- **Part size**: auto-calculated — defaults to 100 MB; files smaller than 100 MB use their actual size, minimum 5 MB. Override with `--part-size`.
- **Workers**: 5 goroutines by default, configurable with `-j`.
- **Retry**: up to 5 attempts per part on 5xx errors, exponential backoff (1 → 2 → 4 → 8 → 16 s). 4xx errors are non-retryable.
- **Limits**: max 1 TB per file; max 10,000 parts per upload.
- **Batch retry**: after uploading a batch of files, any failures are offered for interactive retry.
- **Progress**: animated bar at 10 Hz when stderr is a TTY; one line per 10% when piped; link-only on stdout with `--quiet`.

---

### Interactive mode

Run `barfi` without arguments to open the full TUI:

```bash
barfi
```

Interactive mode provides:

- **Upload files** — pick files and destination folder through menus; asks about recursive mode for directories
- **Manage Buzzheavier** — browse, create, rename, move, and delete remote folders; manage bookmarks; edit file notes; batch-select and batch-delete items
- **Library** — link local folders to remote folders for recurring uploads; shows content preview (up to 15 items per section); sync local paths against the server
- **Manage Profiles** — create, edit, switch between, and delete named configuration profiles (e.g. personal and work accounts)

WSL2 users: Windows paths (e.g. `C:\Users\foo`) are automatically normalized to `/mnt/c/Users/foo`.

---

### Flag reference

| Flag | Short | Description |
|------|-------|-------------|
| `--server URL` | | Server base URL |
| `--token T` | | Authentication token |
| `--location-id ID` | `-l` | Storage bucket ID (where files are physically stored) |
| `--parent-id ID` | `-d` | Target folder ID (where files appear in your file tree) |
| `--guest-upload-link-id ID` | | Guest upload link ID (no token required) |
| `--note TEXT` | | Upload note (max 500 chars) |
| `--part-size BYTES` | | Per-part size override (e.g. `25MB`; range: 5 MB – 100 MB) |
| `--workers N` | `-j` | Parallel upload goroutines (default: 5) |
| `--recursive` | `-r` | Upload directories recursively |
| `--quiet` | `-q` | Suppress progress; print only the link on stdout |
| `--json` | | Print raw server response as JSON |
| `--save` | | Persist the current resolved settings to the active profile |
| `--config ACTION` | | Manage config: `show`, `set KEY VALUE`, `unset KEY` |
| `--version` | | Print version |
| `--help` | `-h` | Print help |

Environment variables (lowest precedence after config file): `BARFI_SERVER`, `BARFI_TOKEN`, `BARFI_LOCATION_ID`.

Precedence: `flags > env vars > config file`.

---

### Configuration

Stored at `~/.config/barfi/config.json` (mode `0600`).

```bash
barfi --config show
barfi --config set server https://buzzheavier.com
barfi --config set token YOUR_TOKEN
barfi --config set workers 10
barfi --config set parentId FOLDER_ID      # default destination folder
barfi --config set locationId BUCKET_ID    # default storage bucket
barfi --config unset token
```

Valid keys for `--config set/unset`: `server`, `token`, `locationId`, `parentId`, `workers`.

**Multiple profiles** are managed through interactive mode ("Manage Profiles" menu). Each profile stores its own `server`, `token`, `parentId`, `locationId`, `workers`, and `defaultNote`. Profiles are useful for separating personal and work accounts or different servers.

Old flat configs (pre-0.1.0) are automatically migrated to profile format on first load.

---

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Upload failed |
| `2` | Usage error |
| `130` | Interrupted (Ctrl+C) |

---

### API reference

[docs/buzzheavier-api.md](docs/buzzheavier-api.md) — full buzzheavier.com API reference (endpoints, parameters, curl examples, known discrepancies).
