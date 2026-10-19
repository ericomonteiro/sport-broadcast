// Package config loads the application settings.
//
// Settings come from a properties file and from environment variables. The
// file is chosen by the ENVIRONMENT variable: ENVIRONMENT=local loads
// application-local.properties. Environment variables override the file.
//
// Keys use dots in the file (db.host) and the same name in upper case with
// underscores in the environment (DB_HOST).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config holds every setting the application needs at runtime.
type Config struct {
	Environment     string
	ServerAddr      string
	DBHost          string
	DBPort          string
	DBUser          string
	DBPassword      string
	DBName          string
	DBSSLMode       string
	OBSAddr         string
	OBSPassword     string
	OBSReplayScene  string
	OBSReplaySource string
	OBSReplayIntro  string
}

// DatabaseURL builds the Postgres connection URL.
func (c Config) DatabaseURL() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     c.DBHost + ":" + c.DBPort,
		Path:     c.DBName,
		RawQuery: url.Values{"sslmode": {c.DBSSLMode}}.Encode(),
	}
	return u.String()
}

// Load reads the settings for the environment named by ENVIRONMENT.
func Load() (Config, error) {
	env := os.Getenv("ENVIRONMENT")

	props := map[string]string{}
	if env != "" {
		path := filepath.Join(".", "application-"+env+".properties")
		loaded, err := ReadProperties(path)
		if err != nil {
			return Config{}, err
		}
		props = loaded
	}

	lookup := func(key string) string {
		if v, ok := os.LookupEnv(envName(key)); ok {
			return v
		}
		return props[key]
	}

	cfg := Config{
		Environment:     env,
		ServerAddr:      valueOr(lookup("server.addr"), ":8080"),
		DBHost:          lookup("db.host"),
		DBPort:          lookup("db.port"),
		DBUser:          lookup("db.user"),
		DBPassword:      lookup("db.password"),
		DBName:          lookup("db.name"),
		DBSSLMode:       valueOr(lookup("db.sslmode"), "disable"),
		OBSAddr:         valueOr(lookup("obs.addr"), "localhost:4455"),
		OBSPassword:     lookup("obs.password"),
		OBSReplayScene:  valueOr(lookup("obs.replay.scene"), "Replay"),
		OBSReplaySource: valueOr(lookup("obs.replay.source"), "Replay Clip"),
		OBSReplayIntro:  valueOr(lookup("obs.replay.intro"), "Replay Intro"),
	}

	var missing []string
	for key, value := range map[string]string{
		"db.host": cfg.DBHost,
		"db.port": cfg.DBPort,
		"db.user": cfg.DBUser,
		"db.name": cfg.DBName,
	} {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required settings: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// ReadProperties parses a .properties file: "key=value" lines, with "#" or "!"
// starting a comment. Blank lines are ignored.
func ReadProperties(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("properties file not found: %s", path)
		}
		return nil, fmt.Errorf("opening properties file: %w", err)
	}
	defer file.Close()

	props := map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("%s:%d: expected key=value", path, lineNumber)
		}
		props[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return props, nil
}

func envName(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
