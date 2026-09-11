package config

import (
	"bufio"
	"os"
	"strings"
)

// loadDotEnv reads a simple KEY=VALUE file and sets any variables that are not
// already present in the process environment (so real environment variables
// always win over the file). It is best-effort: a missing file is not an error.
//
// Supported syntax: blank lines and lines starting with '#' are ignored; the
// value may be wrapped in matching single or double quotes.
func loadDotEnv(path string) {
	f, err := os.Open(path) //nolint:gosec // path is a fixed local config file
	if err != nil {
		return // no .env file — that's fine
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
