package tsp

import "testing"

func TestConfigNormalize(t *testing.T) {
	cfg := Config{
		Tags: []string{
			"",
			" tag:docker ",
			"   ",
			"tag:prod",
		},
	}

	cfg.Normalize()

	want := []string{"tag:docker", "tag:prod"}
	if len(cfg.Tags) != len(want) {
		t.Fatalf("got %v, want %v", cfg.Tags, want)
	}

	for i := range want {
		if cfg.Tags[i] != want[i] {
			t.Fatalf("got %v, want %v", cfg.Tags, want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	t.Run("valid config passes", func(t *testing.T) {
		cfg := Config{
			Tailnet:           "example.github",
			OAuthClientID:     "client-id",
			OAuthClientSecret: "client-secret",
			SwarmNetwork:      "ts-ingress",
			Tags:              []string{"tag:docker", "tag:prod"},
		}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("validate returned error: %v", err)
		}
	})

	t.Run("missing required field fails", func(t *testing.T) {
		cfg := Config{
			OAuthClientID:     "client-id",
			OAuthClientSecret: "client-secret",
			SwarmNetwork:      "ts-ingress",
		}

		if err := cfg.Validate(); err == nil {
			t.Fatal("validate returned nil error")
		}
	})

	t.Run("tag without prefix fails", func(t *testing.T) {
		cfg := Config{
			Tailnet:           "example.github",
			OAuthClientID:     "client-id",
			OAuthClientSecret: "client-secret",
			SwarmNetwork:      "ts-ingress",
			Tags:              []string{"docker"},
		}

		if err := cfg.Validate(); err == nil {
			t.Fatal("validate returned nil error")
		}
	})
}
