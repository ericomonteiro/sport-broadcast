package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// inDir runs the test from a temporary directory so the properties file is found.
func inDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func TestReadPropertiesParsesCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.properties", "# comment\n! also comment\n\nkey.one = value one\nkey.two=two=with-equals\n")

	props, err := ReadProperties(filepath.Join(dir, "p.properties"))
	if err != nil {
		t.Fatalf("ReadProperties: %v", err)
	}
	if props["key.one"] != "value one" {
		t.Fatalf("expected trimmed value, got %q", props["key.one"])
	}
	if props["key.two"] != "two=with-equals" {
		t.Fatalf("expected value to keep later '=', got %q", props["key.two"])
	}
}

func TestReadPropertiesRejectsLineWithoutEquals(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.properties", "not a pair\n")

	if _, err := ReadProperties(filepath.Join(dir, "bad.properties")); err == nil {
		t.Fatalf("expected an error for a line without '='")
	}
}

func TestLoadWithoutEnvironmentUsesOnlyEnvVars(t *testing.T) {
	inDir(t)
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("DB_HOST", "db.example")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_NAME", "n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBHost != "db.example" || cfg.DBPort != "5433" || cfg.ServerAddr != ":8080" || cfg.DBSSLMode != "disable" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadLocalReadsPropertiesFile(t *testing.T) {
	dir := inDir(t)
	writeFile(t, dir, "application-local.properties", strings.Join([]string{
		"server.addr=:9090",
		"db.host=localhost",
		"db.port=5432",
		"db.user=broadcast",
		"db.password=s3cret",
		"db.name=broadcast",
	}, "\n"))
	t.Setenv("ENVIRONMENT", "local")
	os.Unsetenv("DB_PASSWORD") // the file value must be used, not a stray env var

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Environment != "local" || cfg.ServerAddr != ":9090" || cfg.DBPassword != "s3cret" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestEnvVarOverridesPropertiesFile(t *testing.T) {
	dir := inDir(t)
	writeFile(t, dir, "application-local.properties", "db.host=localhost\ndb.port=5432\ndb.user=u\ndb.name=n\n")
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DB_HOST", "from-env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBHost != "from-env" {
		t.Fatalf("expected env var to win, got %q", cfg.DBHost)
	}
}

func TestLoadLocalFailsWhenFileIsMissing(t *testing.T) {
	inDir(t)
	t.Setenv("ENVIRONMENT", "local")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "application-local.properties") {
		t.Fatalf("expected a missing-file error naming application-local.properties, got %v", err)
	}
}

func TestLoadReportsMissingRequiredSettings(t *testing.T) {
	inDir(t)
	t.Setenv("ENVIRONMENT", "")
	os.Unsetenv("DB_HOST")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "db.host") {
		t.Fatalf("expected error naming db.host, got %v", err)
	}
}

func TestDatabaseURLEscapesPassword(t *testing.T) {
	cfg := Config{DBHost: "localhost", DBPort: "5432", DBUser: "u", DBPassword: "p@ss/word", DBName: "broadcast", DBSSLMode: "disable"}

	want := "postgres://u:p%40ss%2Fword@localhost:5432/broadcast?sslmode=disable"
	if got := cfg.DatabaseURL(); got != want {
		t.Fatalf("DatabaseURL = %q, want %q", got, want)
	}
}

func TestOBSSettingsDefaultAndEnvOverride(t *testing.T) {
	inDir(t)
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_NAME", "n")
	os.Unsetenv("OBS_ADDR")
	t.Setenv("OBS_PASSWORD", "from-env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OBSAddr != "localhost:4455" || cfg.OBSPassword != "from-env" {
		t.Fatalf("unexpected OBS config: addr=%q password=%q", cfg.OBSAddr, cfg.OBSPassword)
	}
}
