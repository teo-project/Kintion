// Package config загружает настройки Kintion.
//
// Приоритет (F-26): значения по умолчанию → YAML-файл из TEO_KIN_CONFIG →
// переменные окружения TEO_KIN_*. Секреты можно передать файлом:
// TEO_KIN_<ИМЯ>_FILE (Docker secrets). Переменные без префикса TEO_KIN_
// не читаются.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	packstorage "kintion/pack-storage"
)

// EnvPrefix — префикс всех переменных окружения приложения.
const EnvPrefix = "TEO_KIN_"

// Config — настройки приложения.
type Config struct {
	HTTPBind    string `yaml:"http_bind"`
	LogLevel    string `yaml:"log_level"`
	PublicURL   string `yaml:"public_url"`
	StoragePath string `yaml:"storage_path"`
	DatabaseURL string `yaml:"database_url"`
	Language    string `yaml:"language"`
	// Packs — включённые встроенные паки через запятую; зависимости из requires
	// добавляются сами (Д-31).
	Packs string `yaml:"packs"`
	// PacksPath — читать паки с диска (<путь>/<язык>/<пак>/…) вместо встроенных (Д-5).
	PacksPath string `yaml:"packs_path"`
	// AuthMode — режим входа (F-27); в 0.1 только local (Д-6).
	AuthMode string `yaml:"auth_mode"`
	// Registration — регистрация аккаунтов (F-28); в 0.1 только closed (Д-6).
	Registration string `yaml:"registration"`
	// UploadMaxMB — предел размера загружаемого файла в мегабайтах.
	UploadMaxMB string `yaml:"upload_max_mb"`
}

// Defaults — значения по умолчанию.
func Defaults() Config {
	return Config{
		HTTPBind:    ":8080",
		LogLevel:    "info",
		StoragePath: "/data",
		Language:    "en",
		Packs:       packstorage.DefaultPacks,
		// F-28 по умолчанию invite, но в 0.1 есть только владелец (Д-6)
		AuthMode:     "local",
		Registration: "closed",
		UploadMaxMB:  "20",
	}
}

// field описывает одну настройку: имя переменной (без префикса) и где хранится.
type field struct {
	name   string
	secret bool // разрешён ли <NAME>_FILE
	ptr    func(*Config) *string
}

var fields = []field{
	{name: "HTTP_BIND", ptr: func(c *Config) *string { return &c.HTTPBind }},
	{name: "LOG_LEVEL", ptr: func(c *Config) *string { return &c.LogLevel }},
	{name: "PUBLIC_URL", ptr: func(c *Config) *string { return &c.PublicURL }},
	{name: "STORAGE_PATH", ptr: func(c *Config) *string { return &c.StoragePath }},
	{name: "DATABASE_URL", secret: true, ptr: func(c *Config) *string { return &c.DatabaseURL }},
	{name: "LANGUAGE", ptr: func(c *Config) *string { return &c.Language }},
	{name: "PACKS", ptr: func(c *Config) *string { return &c.Packs }},
	{name: "PACKS_PATH", ptr: func(c *Config) *string { return &c.PacksPath }},
	{name: "AUTH_MODE", ptr: func(c *Config) *string { return &c.AuthMode }},
	{name: "REGISTRATION", ptr: func(c *Config) *string { return &c.Registration }},
	{name: "UPLOAD_MAX_MB", ptr: func(c *Config) *string { return &c.UploadMaxMB }},
}

// LookupFunc — источник переменных окружения (os.LookupEnv в рабочем коде).
type LookupFunc func(key string) (string, bool)

// ReadFileFunc — чтение файлов (os.ReadFile в рабочем коде).
type ReadFileFunc func(name string) ([]byte, error)

// Load читает конфигурацию из окружения процесса.
func Load() (Config, error) {
	return LoadFrom(os.LookupEnv, os.ReadFile)
}

// LoadFrom читает конфигурацию из заданных источников.
func LoadFrom(lookup LookupFunc, readFile ReadFileFunc) (Config, error) {
	cfg := Defaults()

	if path, ok := lookup(EnvPrefix + "CONFIG"); ok && path != "" {
		data, err := readFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("файл конфигурации %s: %w", EnvPrefix+"CONFIG", err)
		}
		if err := applyYAML(&cfg, data); err != nil {
			return Config{}, fmt.Errorf("файл конфигурации %s: %w", path, err)
		}
	}

	for _, f := range fields {
		env := EnvPrefix + f.name
		value, hasValue := lookup(env)
		file, hasFile := "", false
		if f.secret {
			file, hasFile = lookup(env + "_FILE")
		}
		switch {
		case hasValue && hasFile:
			return Config{}, fmt.Errorf("заданы обе переменные %s и %s_FILE", env, env)
		case hasFile:
			data, err := readFile(file)
			if err != nil {
				return Config{}, fmt.Errorf("%s_FILE: %w", env, err)
			}
			*f.ptr(&cfg) = strings.TrimRight(string(data), "\r\n")
		case hasValue:
			*f.ptr(&cfg) = value
		}
	}

	// просто порт («18080») — это «:18080»
	if p, err := strconv.Atoi(cfg.HTTPBind); err == nil && p > 0 {
		cfg.HTTPBind = ":" + cfg.HTTPBind
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyYAML(cfg *Config, data []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// Validate проверяет значения.
func (c Config) Validate() error {
	if c.HTTPBind == "" {
		return errors.New(EnvPrefix + "HTTP_BIND: пустое значение")
	}
	if _, err := ParseLogLevel(c.LogLevel); err != nil {
		return err
	}
	if c.Language == "" {
		return errors.New(EnvPrefix + "LANGUAGE: пустое значение")
	}
	switch c.AuthMode {
	case "local":
	case "both", "oidc":
		return fmt.Errorf("%sAUTH_MODE=%s не поддерживается в 0.1: доступен только local (вход по логину и паролю)", EnvPrefix, c.AuthMode)
	default:
		return fmt.Errorf("%sAUTH_MODE: неизвестный режим %q (local, both, oidc)", EnvPrefix, c.AuthMode)
	}
	switch c.Registration {
	case "closed":
	case "open", "invite":
		return fmt.Errorf("%sREGISTRATION=%s не поддерживается в 0.1: аккаунт один — владелец (closed)", EnvPrefix, c.Registration)
	default:
		return fmt.Errorf("%sREGISTRATION: неизвестное значение %q (open, invite, closed)", EnvPrefix, c.Registration)
	}
	if len(c.PackList()) == 0 {
		return errors.New(EnvPrefix + "PACKS: нужен хотя бы один пак")
	}
	if n, err := strconv.Atoi(c.UploadMaxMB); err != nil || n <= 0 || n > 1024 {
		return fmt.Errorf("%sUPLOAD_MAX_MB: ожидается целое число мегабайт 1…1024, получено %q", EnvPrefix, c.UploadMaxMB)
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%sPUBLIC_URL: ожидается http(s)://хост, получено %q", EnvPrefix, c.PublicURL)
		}
	}
	return nil
}

// UploadMaxBytes — предел размера загружаемого файла в байтах (значение проверено Validate).
func (c Config) UploadMaxBytes() int64 {
	n, _ := strconv.Atoi(c.UploadMaxMB)
	return int64(n) << 20
}

// PackList — список включённых паков из Packs.
func (c Config) PackList() []string {
	var out []string
	for p := range strings.SplitSeq(c.Packs, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseLogLevel переводит строку уровня журнала в slog.Level.
func ParseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("%sLOG_LEVEL: неизвестный уровень %q (debug, info, warn, error)", EnvPrefix, s)
}
