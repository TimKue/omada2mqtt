package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := `# a comment
OMADA_TEST_PLAIN=value1

OMADA_TEST_QUOTED="quoted value"
OMADA_TEST_SINGLE='single value'
  OMADA_TEST_SPACED = spaced
OMADA_TEST_EXISTING=from-file
not a valid line
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// A pre-existing env var must win over the file.
	t.Setenv("OMADA_TEST_EXISTING", "from-env")

	for _, k := range []string{"OMADA_TEST_PLAIN", "OMADA_TEST_QUOTED", "OMADA_TEST_SINGLE", "OMADA_TEST_SPACED"} {
		t.Cleanup(func() { _ = os.Unsetenv(k) })
	}

	loadDotEnv(path)

	cases := map[string]string{
		"OMADA_TEST_PLAIN":    "value1",
		"OMADA_TEST_QUOTED":   "quoted value",
		"OMADA_TEST_SINGLE":   "single value",
		"OMADA_TEST_SPACED":   "spaced",
		"OMADA_TEST_EXISTING": "from-env", // not overridden
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoadDotEnvMissingFileIsNoError(t *testing.T) {
	// Must not panic or fail for a nonexistent file.
	loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist"))
}
