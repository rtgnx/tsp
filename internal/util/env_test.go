package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvValue(t *testing.T) {
	t.Run("missing uses fallback", func(t *testing.T) {
		const name = "TSP_TEST_ENV_MISSING"
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}

		got := EnvValue(name, "fallback")
		if got != "fallback" {
			t.Fatalf("got %q, want %q", got, "fallback")
		}
	})

	t.Run("plain value is returned", func(t *testing.T) {
		const name = "TSP_TEST_ENV_PLAIN"
		t.Setenv(name, "plain-value")

		got := EnvValue(name, "fallback")
		if got != "plain-value" {
			t.Fatalf("got %q, want %q", got, "plain-value")
		}
	})

	t.Run("file value is read", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "secret.txt")
		if err := os.WriteFile(path, []byte("from-file"), 0600); err != nil {
			t.Fatal(err)
		}

		const name = "TSP_TEST_ENV_FILE"
		t.Setenv(name, "file:"+path)

		got := EnvValue(name, "fallback")
		if got != "from-file" {
			t.Fatalf("got %q, want %q", got, "from-file")
		}
	})

	t.Run("unreadable file uses fallback", func(t *testing.T) {
		const name = "TSP_TEST_ENV_BAD_FILE"
		t.Setenv(name, "file:/definitely/missing/path")

		got := EnvValue(name, "fallback")
		if got != "fallback" {
			t.Fatalf("got %q, want %q", got, "fallback")
		}
	})
}
