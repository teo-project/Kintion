package config

import (
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

func env(m map[string]string) LookupFunc {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func files(m map[string]string) ReadFileFunc {
	return func(name string) ([]byte, error) {
		v, ok := m[name]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(v), nil
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := LoadFrom(env(nil), files(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Defaults() {
		t.Fatalf("got %+v, want defaults %+v", cfg, Defaults())
	}
	if cfg.Language != "en" {
		t.Fatalf("default language: %q", cfg.Language)
	}
}

func TestPriority(t *testing.T) {
	yamlFile := "http_bind: \":9000\"\nlog_level: debug\nlanguage: ru\n"
	fsys := files(map[string]string{"/etc/kintion.yml": yamlFile})

	// файл перекрывает значения по умолчанию
	cfg, err := LoadFrom(env(map[string]string{"TEO_KIN_CONFIG": "/etc/kintion.yml"}), fsys)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPBind != ":9000" || cfg.LogLevel != "debug" || cfg.Language != "ru" {
		t.Fatalf("file not applied: %+v", cfg)
	}
	if cfg.StoragePath != Defaults().StoragePath {
		t.Fatalf("default lost: %+v", cfg)
	}

	// окружение перекрывает файл
	cfg, err = LoadFrom(env(map[string]string{
		"TEO_KIN_CONFIG":    "/etc/kintion.yml",
		"TEO_KIN_HTTP_BIND": ":7000",
	}), fsys)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPBind != ":7000" || cfg.LogLevel != "debug" {
		t.Fatalf("env must override file: %+v", cfg)
	}
}

func TestSecretFile(t *testing.T) {
	fsys := files(map[string]string{
		"/run/secrets/db": "postgres://u:p@db/kintion\n",
		"/etc/k.yml":      "database_url: postgres://from-file\n",
	})

	cfg, err := LoadFrom(env(map[string]string{
		"TEO_KIN_CONFIG":            "/etc/k.yml",
		"TEO_KIN_DATABASE_URL_FILE": "/run/secrets/db",
	}), fsys)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://u:p@db/kintion" {
		t.Fatalf("_FILE must override file and trim newline: %q", cfg.DatabaseURL)
	}

	_, err = LoadFrom(env(map[string]string{
		"TEO_KIN_DATABASE_URL":      "postgres://x",
		"TEO_KIN_DATABASE_URL_FILE": "/run/secrets/db",
	}), fsys)
	if err == nil {
		t.Fatal("both value and _FILE must be an error")
	}

	_, err = LoadFrom(env(map[string]string{"TEO_KIN_DATABASE_URL_FILE": "/missing"}), fsys)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing secret file: %v", err)
	}
}

func TestFileOnlyForSecrets(t *testing.T) {
	// _FILE работает только для секретов; для остальных переменная игнорируется
	cfg, err := LoadFrom(env(map[string]string{"TEO_KIN_HTTP_BIND_FILE": "/x"}), files(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPBind != Defaults().HTTPBind {
		t.Fatalf("non-secret _FILE must be ignored: %+v", cfg)
	}
}

func TestOnlyPrefixedVariables(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"HTTP_BIND":    ":1",
		"LOG_LEVEL":    "debug",
		"DATABASE_URL": "postgres://unprefixed",
		"PORT":         "1",
		"KIN_LANGUAGE": "en",
		"CONFIG":       "/etc/k.yml",
	}), files(map[string]string{"/etc/k.yml": "http_bind: \":2\"\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Defaults() {
		t.Fatalf("unprefixed variables must be ignored: %+v", cfg)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]map[string]string{
		"log level":  {"TEO_KIN_LOG_LEVEL": "loud"},
		"public url": {"TEO_KIN_PUBLIC_URL": "kintion.example"},
		"empty bind": {"TEO_KIN_HTTP_BIND": ""},
		"language":   {"TEO_KIN_LANGUAGE": ""},
	}
	for name, e := range cases {
		if _, err := LoadFrom(env(e), files(nil)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestUnknownYAMLKey(t *testing.T) {
	_, err := LoadFrom(
		env(map[string]string{"TEO_KIN_CONFIG": "/k.yml"}),
		files(map[string]string{"/k.yml": "http_bnd: \":1\"\n"}),
	)
	if err == nil || !strings.Contains(err.Error(), "http_bnd") {
		t.Fatalf("unknown key must be an error, got %v", err)
	}
}

func TestEmptyYAML(t *testing.T) {
	cfg, err := LoadFrom(
		env(map[string]string{"TEO_KIN_CONFIG": "/k.yml"}),
		files(map[string]string{"/k.yml": ""}),
	)
	if err != nil || cfg != Defaults() {
		t.Fatalf("empty file: %+v %v", cfg, err)
	}
}

// Д-6: в 0.1 только local и closed; иное — ошибка старта с понятным текстом.
func TestAuthModeUnsupported(t *testing.T) {
	for k, v := range map[string]string{
		"TEO_KIN_AUTH_MODE":    "oidc",
		"TEO_KIN_REGISTRATION": "invite",
	} {
		_, err := LoadFrom(env(map[string]string{k: v}), files(nil))
		if err == nil || !strings.Contains(err.Error(), "не поддерживается в 0.1") {
			t.Errorf("%s=%s: %v", k, v, err)
		}
	}
}

// TEO_KIN_HTTP_BIND принимает и адрес, и просто порт.
func TestHTTPBindPort(t *testing.T) {
	for in, want := range map[string]string{"18080": ":18080", ":18080": ":18080", "127.0.0.1:18080": "127.0.0.1:18080"} {
		cfg, err := LoadFrom(env(map[string]string{"TEO_KIN_HTTP_BIND": in}), files(nil))
		if err != nil || cfg.HTTPBind != want {
			t.Errorf("%q: got %q, %v; want %q", in, cfg.HTTPBind, err, want)
		}
	}
}

// TestEnvExampleDescribesAllVariables — сторож F-134: каждая TEO_KIN_*, которую читает конфиг,
// описана в deploy/.env.example (раскомментированной или закомментированной строкой).
func TestEnvExampleDescribesAllVariables(t *testing.T) {
	data, err := os.ReadFile("../../../deploy/.env.example")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{EnvPrefix + "CONFIG"}
	for _, f := range fields {
		names = append(names, EnvPrefix+f.name)
		if f.secret {
			names = append(names, EnvPrefix+f.name+"_FILE")
		}
	}
	for _, name := range names {
		re := regexp.MustCompile(`(?m)^#?\s*` + regexp.QuoteMeta(name) + `=`)
		if !re.Match(data) {
			t.Errorf(".env.example не описывает %s", name)
		}
	}
}
