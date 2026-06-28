package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
)

func eprintf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
}

func eprintln(args ...any) {
	_, _ = fmt.Fprintln(os.Stderr, args...)
}

const version = "0.1.0"

type cliOptions struct {
	server       string
	token        string
	locationId   string
	parentId     string
	guestLink    string
	note         string
	partSizeStr  string
	workers      int
	configAction string // "show", "set", or "unset"
	save         bool
	quiet        bool
	jsonOutput   bool
	showHelp     bool
	showVersion  bool
	recursive    bool

	files      []string
	configArgs []string // remaining positional args when --config is active
}

func parseFlags(args []string) (*cliOptions, error) {
	fs := flag.NewFlagSet("barfi", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	opts := &cliOptions{}
	fs.StringVar(&opts.server, "server", "", "BUS server base URL")
	fs.StringVar(&opts.token, "token", "", "Bearer token")
	fs.StringVar(&opts.locationId, "location-id", "", "Storage bucket id")
	fs.StringVar(&opts.locationId, "l", "", "Storage bucket id (short)")
	fs.StringVar(&opts.parentId, "parent-id", "", "Target directory id")
	fs.StringVar(&opts.parentId, "d", "", "Target directory id (short)")
	fs.StringVar(&opts.guestLink, "guest-upload-link-id", "", "Guest upload link id")
	fs.StringVar(&opts.note, "note", "", "Optional note, ≤500 chars")
	fs.StringVar(&opts.partSizeStr, "part-size", "", "Override auto part size (e.g. 25MB)")
	fs.IntVar(&opts.workers, "workers", 0, "Parallel upload workers (default 5)")
	fs.IntVar(&opts.workers, "j", 0, "Parallel upload workers (short)")
	fs.StringVar(&opts.configAction, "config", "", "Config management: show, set <key> <value>, unset <key>")
	fs.BoolVar(&opts.save, "save", false, "Persist resolved settings to config file")
	fs.BoolVar(&opts.quiet, "quiet", false, "Suppress progress output")
	fs.BoolVar(&opts.quiet, "q", false, "Suppress progress output (short)")
	fs.BoolVar(&opts.jsonOutput, "json", false, "Print server response as JSON")
	fs.BoolVar(&opts.showVersion, "version", false, "Print version and exit")
	fs.BoolVar(&opts.showHelp, "help", false, "Print help and exit")
	fs.BoolVar(&opts.showHelp, "h", false, "Print help and exit (short)")
	fs.BoolVar(&opts.recursive, "recursive", false, "Upload de diretórios de forma recursiva")
	fs.BoolVar(&opts.recursive, "r", false, "Upload de diretórios de forma recursiva (short)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	rest := fs.Args()
	if opts.configAction != "" {
		// --config mode: positional args are key/value for set/unset, not a file path.
		opts.configArgs = rest
	} else {
		opts.files = rest
	}
	return opts, nil
}

// resolveSettings merges config file, env vars, and CLI flags with the
// precedence: config < env < flags.
func resolveSettings(opts *cliOptions) (Config, MultiConfig, error) {
	var cfg Config
	var mCfg MultiConfig
	path, err := defaultConfigPath()
	if err == nil {
		loaded, lerr := loadConfig(path)
		if lerr != nil {
			return cfg, mCfg, lerr
		}
		mCfg = loaded
		if mCfg.Profiles == nil {
			mCfg.Profiles = make(map[string]Config)
		}
		cfg = mCfg.Profiles[mCfg.ActiveProfile]

		// Auto-fix para usuários que colaram o ID da pasta na Location (Bucket)
		if cfg.LocationId != "" && cfg.ParentId == "" {
			cfg.ParentId = cfg.LocationId
			cfg.LocationId = ""
			mCfg.Profiles[mCfg.ActiveProfile] = cfg
			_ = saveConfig(path, mCfg) // Salva a correção silenciosamente
		}
	} else {
		mCfg = MultiConfig{
			ActiveProfile: "Padrão",
			Profiles: map[string]Config{
				"Padrão": {},
			},
		}
	}

	if v := os.Getenv("BARFI_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("BARFI_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("BARFI_LOCATION_ID"); v != "" {
		cfg.LocationId = v
	}
	if opts.server != "" {
		cfg.Server = opts.server
	}
	if opts.token != "" {
		cfg.Token = opts.token
	}
	if opts.locationId != "" {
		cfg.LocationId = opts.locationId
	}
	if opts.parentId != "" {
		cfg.ParentId = opts.parentId
	}
	if opts.workers > 0 {
		cfg.Workers = opts.workers
	}
	if cfg.Workers == 0 {
		cfg.Workers = 5
	}
	return cfg, mCfg, nil
}

func printHelp() {
	eprintln(`Uso: barfi [flags] [arquivos...]
	
Se nenhum arquivo for passado, o menu interativo será aberto.

Flags:
  --server URL               URL base do servidor (env: BARFI_SERVER)
  --token T                  Token de autorização (env: BARFI_TOKEN)
  -l, --location-id ID       ID do bucket de armazenamento (env: BARFI_LOCATION_ID)
  -d, --parent-id ID         ID do diretório de destino (requer --token)
      --guest-upload-link-id ID do link de upload para convidado
      --note TEXT            Nota opcional (≤500 caracteres)
      --part-size BYTES      Substituir tamanho automático da parte (ex: 25MB)
  -j, --workers N            Workers de upload paralelo (padrão 5)
  -r, --recursive            Faz upload dos arquivos em subdiretórios
      --save                 Salvar configurações resolvidas no arquivo de config
      --config ACTION        Gerenciamento de config (show, set <chave> <valor>, unset <chave>)
  -q, --quiet                Suprimir saída de progresso
      --json                 Imprimir resposta do servidor em JSON ao concluir
  -h, --help                 Imprimir ajuda e sair
      --version              Imprimir versão e sair

Chaves de config: server, token, locationId, parentId, workers`)
}

func expandFiles(paths []string, recursive bool) ([]string, error) {
	var expanded []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("erro acessando %q: %w", p, err)
		}
		if !info.IsDir() {
			expanded = append(expanded, p)
			continue
		}

		if recursive {
			err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					expanded = append(expanded, path)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("erro percorrendo diretório %q: %w", p, err)
			}
		} else {
			entries, err := os.ReadDir(p)
			if err != nil {
				return nil, fmt.Errorf("erro lendo diretório %q: %w", p, err)
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					expanded = append(expanded, filepath.Join(p, entry.Name()))
				}
			}
		}
	}
	return expanded, nil
}

func runCLI(args []string) int {
	opts, err := parseFlags(args)
	if err != nil {
		eprintln("barfi:", err)
		return 2
	}
	if opts.showHelp {
		printHelp()
		return 0
	}
	if opts.showVersion {
		fmt.Println("barfi", version)
		return 0
	}

	if opts.configAction != "" {
		return runConfig(opts.configAction, opts.configArgs)
	}

	cfg, mCfg, err := resolveSettings(opts)
	if err != nil {
		eprintln("barfi:", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fromInteractive := len(opts.files) == 0 && !opts.save
	for {
		if len(opts.files) == 0 && !opts.save {
			err := runInteractiveMode(opts, &mCfg, &cfg)
			if err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					return 0
				}
				eprintln("barfi: erro no modo interativo:", err)
				return 2
			}
			if len(opts.files) == 0 {
				return 0
			}
		} else if opts.note == "" {
			// Caso não seja interativo e não tenha passado nota por flag, usa a do perfil
			opts.note = cfg.DefaultNote
		}

		if opts.save {
			mCfg.Profiles[mCfg.ActiveProfile] = cfg
			path, perr := defaultConfigPath()
			if perr != nil {
				eprintln("barfi:", perr)
				return 2
			}
			if err := saveConfig(path, mCfg); err != nil {
				eprintln("barfi:", err)
				return 2
			}
			if cfg.Token != "" && !opts.quiet {
				eprintf("barfi: perfil %q atualizado em %s (modo 0600)\n", mCfg.ActiveProfile, path)
			}
			if len(opts.files) == 0 {
				return 0
			}
		}

		if len(opts.files) == 0 {
			eprintln("barfi: nenhum arquivo selecionado (use --help para ajuda)")
			return 2
		}
		if cfg.Server == "" {
			eprintln("barfi: --server não configurado (use uma flag, variável de ambiente BARFI_SERVER, ou use o modo interativo)")
			return 2
		}

		var partSize int64
		if opts.partSizeStr != "" {
			n, perr := parseSize(opts.partSizeStr)
			if perr != nil {
				eprintln("barfi:", perr)
				return 2
			}
			if n < MinPartSize {
				if !opts.quiet {
					eprintf("barfi: --part-size %s abaixo do mínimo, usando %s\n", opts.partSizeStr, humanSize(MinPartSize))
				}
				n = MinPartSize
			}
			if n > MaxPartSize {
				if !opts.quiet {
					eprintf("barfi: --part-size %s acima do máximo, usando %s\n", opts.partSizeStr, humanSize(MaxPartSize))
				}
				n = MaxPartSize
			}
			partSize = n
		}

		filesToUpload, err := expandFiles(opts.files, opts.recursive)
		if err != nil {
			eprintln("barfi:", err)
			return 2
		}

		if len(filesToUpload) == 0 {
			eprintln("barfi: nenhum arquivo encontrado para upload.")
			return 2
		}

		type failedUpload struct {
			path string
			err  error
		}
		var failed []failedUpload
		var successCount int

		for len(filesToUpload) > 0 {
			var retryFiles []string

			for i, filePath := range filesToUpload {
				if !opts.quiet {
					if len(filesToUpload) > 1 {
						eprintf("\n=== Fazendo upload de %d de %d: %s ===\n", i+1, len(filesToUpload), filePath)
					}
				}

				f, err := os.Open(filePath)
				if err != nil {
					eprintf("barfi erro ao abrir %s: %v\n", filePath, err)
					failed = append(failed, failedUpload{filePath, err})
					continue
				}

				st, err := f.Stat()
				if err != nil {
					f.Close()
					eprintf("barfi erro lendo status de %s: %v\n", filePath, err)
					failed = append(failed, failedUpload{filePath, err})
					continue
				}

				fileSize := st.Size()
				fileName := filepathBase(filePath)
				server := strings.TrimRight(cfg.Server, "/")

				effectivePartSize := partSize
				if effectivePartSize == 0 {
					effectivePartSize = calcPartSize(fileSize)
				}
				totalParts := (fileSize + effectivePartSize - 1) / effectivePartSize

				if !opts.quiet {
					eprintf("arquivo:     %s (%s)\n", fileName, humanSize(fileSize))
					eprintf("servidor:    %s\n", server)
					eprintf("partes:      %d x %s\n", totalParts, humanSize(effectivePartSize))
					eprintf("workers:     %d\n", cfg.Workers)
					if cfg.ParentId != "" {
						eprintf("diretório:   %s\n", cfg.ParentId)
					}
					if opts.guestLink != "" {
						eprintf("link convid: %s\n", opts.guestLink)
					}
					if cfg.LocationId != "" {
						eprintf("localização: %s\n", cfg.LocationId)
					}
				}

				u := &Uploader{
					file:        f,
					fileSize:    fileSize,
					fileName:    fileName,
					server:      server,
					token:       cfg.Token,
					locationId:  cfg.LocationId,
					parentId:    cfg.ParentId,
					guestLinkId: opts.guestLink,
					note:        opts.note,
					partSize:    partSize,
					workers:     cfg.Workers,
					httpClient:  &http.Client{Timeout: 0},
					progress:    newProgress(opts.quiet, fileName),
				}

				start := time.Now()
				result, err := u.run(ctx)
				f.Close()

				if err != nil {
					if errors.Is(err, context.Canceled) {
						eprintln("barfi: cancelado")
						return 130
					}

					eprintf("barfi falha no envio de %s\n", fileName)
					formatError(err)
					failed = append(failed, failedUpload{filePath, err})
					continue
				}

				successCount++
				if !opts.quiet {
					elapsed := time.Since(start)
					var avgSpeed string
					if secs := elapsed.Seconds(); secs > 0 {
						avgSpeed = humanSize(int64(float64(u.fileSize)/secs)) + "/s"
					}
					eprintf("enviado %s (%s) em %s (%s)\n",
						u.fileName, humanSize(u.fileSize), elapsed.Round(time.Second), avgSpeed)
				}
				if opts.jsonOutput {
					var buf bytes.Buffer
					if err := json.Indent(&buf, result.rawJSON, "", "  "); err != nil {
						fmt.Println(string(result.rawJSON))
					} else {
						fmt.Println(buf.String())
					}
				} else {
					fmt.Println(result.link)
				}
			}

			if len(failed) > 0 {
				eprintf("\nResumo do Lote:\n")
				eprintf("  Sucesso: %d\n", successCount)
				eprintf("  Falhas:  %d\n", len(failed))

				for _, f := range failed {
					eprintf("    - %s (%v)\n", f.path, f.err)
				}

				var retry bool
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title(fmt.Sprintf("Deseja tentar enviar novamente os %d arquivos que falharam?", len(failed))).
							Value(&retry),
					),
				)
				_ = form.Run()

				if retry {
					for _, f := range failed {
						retryFiles = append(retryFiles, f.path)
					}
					failed = nil
					filesToUpload = retryFiles
					continue
				} else {
					return 1
				}
			} else {
				if len(filesToUpload) > 1 {
					eprintf("\nLote finalizado com sucesso! %d arquivos enviados.\n", successCount)
				}
				break
			}
		}

		if !fromInteractive {
			return 0
		}
		// Volta ao menu principal após upload concluído no modo interativo.
		opts.files = nil
		opts.note = ""
		cfg = mCfg.Profiles[mCfg.ActiveProfile]
	}
}

func runConfig(action string, args []string) int {
	path, err := defaultConfigPath()
	if err != nil {
		eprintln("barfi:", err)
		return 2
	}

	switch action {
	case "show":
		mCfg, err := loadConfig(path)
		if err != nil {
			eprintln("barfi:", err)
			return 1
		}
		data, _ := json.MarshalIndent(mCfg, "", "  ")
		fmt.Println(string(data))
		return 0

	case "set":
		if len(args) != 2 {
			eprintln("barfi: --config set requires <key> <value>")
			return 2
		}
		mCfg, err := loadConfig(path)
		if err != nil {
			eprintln("barfi:", err)
			return 1
		}
		cfg := mCfg.Profiles[mCfg.ActiveProfile]
		if err := setConfigField(&cfg, args[0], args[1]); err != nil {
			eprintln("barfi:", err)
			return 2
		}
		mCfg.Profiles[mCfg.ActiveProfile] = cfg
		if err := saveConfig(path, mCfg); err != nil {
			eprintln("barfi:", err)
			return 1
		}
		return 0

	case "unset":
		if len(args) != 1 {
			eprintln("barfi: --config unset requires <key>")
			return 2
		}
		mCfg, err := loadConfig(path)
		if err != nil {
			eprintln("barfi:", err)
			return 1
		}
		cfg := mCfg.Profiles[mCfg.ActiveProfile]
		if err := setConfigField(&cfg, args[0], ""); err != nil {
			eprintln("barfi:", err)
			return 2
		}
		mCfg.Profiles[mCfg.ActiveProfile] = cfg
		if err := saveConfig(path, mCfg); err != nil {
			eprintln("barfi:", err)
			return 1
		}
		return 0

	default:
		eprintf("barfi: unknown config action %q (use show, set, or unset)\n", action)
		return 2
	}
}

func setConfigField(cfg *Config, key, value string) error {
	switch key {
	case "server":
		cfg.Server = value
	case "token":
		cfg.Token = value
	case "locationId":
		cfg.LocationId = value
	case "parentId":
		cfg.ParentId = value
	case "workers":
		if value == "" {
			cfg.Workers = 0
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("workers must be an integer, got %q", value)
		}
		if n < 1 {
			return fmt.Errorf("workers must be >= 1, got %d", n)
		}
		cfg.Workers = n
	default:
		return fmt.Errorf("unknown config key %q (valid: server, token, locationId, parentId, workers)", key)
	}
	return nil
}

func formatError(err error) int {
	switch {
	case errors.Is(err, context.Canceled):
		eprintln("barfi: cancelled")
		return 130
	case errors.Is(err, errExpired):
		eprintln("barfi: upload session expired — start over")
		return 1
	case errors.Is(err, errPartTooLarge):
		eprintln("barfi: part exceeds server size limit (100 MB)")
		return 1
	default:
		eprintln("barfi:", err)
		return 1
	}
}

// filepathBase is a tiny wrapper so we don't import path/filepath for this one use.
func filepathBase(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	if i < 0 {
		return p
	}
	return p[i+1:]
}

func main() { os.Exit(runCLI(os.Args[1:])) }
