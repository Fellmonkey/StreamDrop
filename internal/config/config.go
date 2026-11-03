// Package config resolves runtime configuration from defaults, a JSON config
// file, environment variables and command-line flags, in ascending priority.
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Version is stamped at build time with -ldflags "-X streamdrop/internal/config.Version=...".
var Version = "1.0.0"

// Config is the effective application configuration.
type Config struct {
	// Port is the preferred UI port. If it is busy the next free port is used.
	Port int `json:"port"`
	// Host is the loopback address the server binds to.
	Host string `json:"host"`
	// DataDir holds the SQLite database, logs and downloaded components.
	DataDir string `json:"data_dir"`
	// NoTray disables the system tray (useful for headless runs and tests).
	NoTray bool `json:"no_tray"`
	// OpenBrowser opens the UI in the default browser at startup.
	OpenBrowser bool `json:"open_browser"`
	// UpdateRepo is the GitHub "owner/repo" used by the self-updater.
	UpdateRepo string `json:"update_repo"`
	// LogLevel is one of debug|info|warn|error.
	LogLevel string `json:"log_level"`
	// OAuthCallbackPort is the fixed loopback port for provider callbacks.
	OAuthCallbackPort int `json:"oauth_callback_port"`

	// ConfigFile is where the loaded config file lives (informational).
	ConfigFile string `json:"-"`
}

// Defaults returns the configuration used when nothing is overridden.
func Defaults() Config {
	return Config{
		Port:              8085,
		Host:              "127.0.0.1",
		DataDir:           defaultDataDir(),
		NoTray:            false,
		OpenBrowser:       true,
		UpdateRepo:        "streamdrop/streamdrop",
		LogLevel:          "info",
		OAuthCallbackPort: 35357,
	}
}

func defaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			if home, err := os.UserHomeDir(); err == nil {
				base = filepath.Join(home, "AppData", "Local")
			}
		}
		return filepath.Join(base, "StreamDrop")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "StreamDrop")
	default:
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "streamdrop")
	}
}

// Load resolves configuration. args are the process arguments without the
// program name.
func Load(args []string) (Config, error) {
	cfg := Defaults()

	// First pass: find --config/--data-dir so the file can be located before
	// the full parse. Errors are intentionally ignored here; the real parse
	// below reports them.
	pre := flag.NewFlagSet("streamdrop-pre", flag.ContinueOnError)
	pre.SetOutput(discard{})
	configPath := pre.String("config", "", "")
	preDataDir := pre.String("data-dir", "", "")
	_ = pre.Parse(args)

	if v := os.Getenv("STREAMDROP_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if *preDataDir != "" {
		cfg.DataDir = *preDataDir
	}
	if *configPath == "" {
		*configPath = filepath.Join(cfg.DataDir, "config.json")
	}
	cfg.ConfigFile = *configPath

	if err := loadFile(*configPath, &cfg); err != nil {
		return cfg, err
	}

	// Environment overrides the file.
	if v := os.Getenv("STREAMDROP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("STREAMDROP_PORT: %w", err)
		}
		cfg.Port = n
	}
	if v := os.Getenv("STREAMDROP_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("STREAMDROP_UPDATE_REPO"); v != "" {
		cfg.UpdateRepo = v
	}
	if v := os.Getenv("STREAMDROP_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("STREAMDROP_NO_TRAY"); v == "1" || strings.EqualFold(v, "true") {
		cfg.NoTray = true
	}

	// Flags override everything.
	fs := flag.NewFlagSet("streamdrop", flag.ContinueOnError)
	fs.SetOutput(discard{})
	port := fs.Int("port", cfg.Port, "preferred HTTP port")
	host := fs.String("host", cfg.Host, "bind address (loopback only)")
	dataDir := fs.String("data-dir", cfg.DataDir, "data directory")
	noTray := fs.Bool("no-tray", cfg.NoTray, "do not create a system tray icon")
	noBrowser := fs.Bool("no-browser", !cfg.OpenBrowser, "do not open a browser at startup")
	updateRepo := fs.String("update-repo", cfg.UpdateRepo, "GitHub owner/repo for updates")
	logLevel := fs.String("log-level", cfg.LogLevel, "debug|info|warn|error")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.StringVar(configPath, "config", *configPath, "config file path")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if *showVersion {
		return cfg, ErrVersionRequested
	}

	cfg.Port = *port
	cfg.Host = *host
	cfg.DataDir = *dataDir
	cfg.NoTray = *noTray
	cfg.OpenBrowser = !*noBrowser
	cfg.UpdateRepo = *updateRepo
	cfg.LogLevel = *logLevel
	cfg.ConfigFile = *configPath

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// ErrVersionRequested signals that --version was passed.
var ErrVersionRequested = errors.New("version requested")

func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	if c.Host != "127.0.0.1" && c.Host != "localhost" && c.Host != "::1" {
		return fmt.Errorf("host must be a loopback address, got %q", c.Host)
	}
	if c.DataDir == "" {
		return errors.New("data directory is empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level %q", c.LogLevel)
	}
	return nil
}

// EnsureDirs creates the data directory layout.
func (c Config) EnsureDirs() error {
	for _, dir := range []string{
		c.DataDir,
		c.LogsDir(),
		c.DepsDir(),
		c.CrashDir(),
		c.UpdateStagingDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) LogsDir() string          { return filepath.Join(c.DataDir, "logs") }
func (c Config) DepsDir() string          { return filepath.Join(c.DataDir, "deps") }
func (c Config) CrashDir() string         { return filepath.Join(c.DataDir, "crash") }
func (c Config) UpdateStagingDir() string { return filepath.Join(c.DataDir, "update", "staging") }
func (c Config) DatabasePath() string     { return filepath.Join(c.DataDir, "streamdrop.db") }

func loadFile(path string, cfg *Config) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}
	// Only the documented keys are accepted; unknown keys are ignored so an
	// older config file keeps working after an upgrade.
	if err := json.Unmarshal(raw, cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	return nil
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
