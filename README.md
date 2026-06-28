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

[Leia em Português](README)

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

# Upload with a note
barfi --note="final cut" archive.zip

# Upload via guest link
barfi --guest-upload-link-id=LINK_ID file.txt

# Capture the link in a script
LINK=$(barfi -q file.txt)
```

---

### Interactive mode

Run `barfi` without arguments to open the full TUI:

```bash
barfi
```

Interactive mode provides:

- **Upload files** — pick files and destination folder through menus
- **Manage Buzzheavier** — browse, create, rename, move, and delete remote folders; manage bookmarks; edit file notes; batch-delete items
- **Library** — link local folders to remote folders for recurring uploads with content preview; sync with the server
- **Manage Profiles** — create and switch between multiple configuration profiles (e.g. personal and work accounts)

---

### Flag reference

| Flag | Short | Description |
|------|-------|-------------|
| `--server URL` | | Server base URL |
| `--token T` | | Authentication token |
| `--location-id ID` | `-l` | Storage bucket ID |
| `--parent-id ID` | `-d` | Target folder ID |
| `--guest-upload-link-id ID` | | Guest upload link ID |
| `--note TEXT` | | Upload note (max 500 chars) |
| `--part-size BYTES` | | Per-part size override (e.g. `25MB`, default: `100MB`) |
| `--workers N` | `-j` | Parallel upload goroutines (default: 5) |
| `--recursive` | `-r` | Upload directories recursively |
| `--quiet` | `-q` | Suppress progress; print only the link on stdout |
| `--json` | | Print raw server response as JSON |
| `--save` | | Persist current resolved settings to config file |
| `--config ACTION` | | Manage config: `show`, `set`, `unset` |
| `--version` | | Print version |
| `--help` | `-h` | Print help |

Environment variables: `BARFI_SERVER`, `BARFI_TOKEN`, `BARFI_LOCATION_ID`.

Precedence: flags > environment variables > config file.

---

### Configuration

Stored at `~/.config/barfi/config.json` (mode `0600`).

```bash
barfi --config show                               # Show current config
barfi --config set server https://buzzheavier.com
barfi --config set token YOUR_TOKEN
barfi --config set workers 10
barfi --config set parent-id FOLDER_ID            # Default destination folder
barfi --config unset token                        # Remove a key
```

Valid keys: `server`, `token`, `locationId`, `parentId`, `workers`.

**Multiple profiles** are managed through interactive mode. Use the "Manage Profiles" menu to create, edit, switch between, and delete profiles.

---

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Upload failed |
| `2` | Usage error |
| `130` | Interrupted (Ctrl+C) |
