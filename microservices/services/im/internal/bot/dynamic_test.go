package bot

import (
	"context"
	"testing"
	"time"
)

func baseConfig() Config {
	return Config{
		Enabled: true,
		BaseURL: "https://env.example.com",
		APIKey:  "env-key",
		Model:   "env-model",
		Timeout: time.Second,
	}
}

func TestDynamicUsesEnvConfigWithoutOverrides(t *testing.T) {
	d := NewDynamic(baseConfig(), func(context.Context) Overrides { return Overrides{} })
	cfg := d.effectiveConfig(context.Background())
	if cfg.BaseURL != "https://env.example.com" || cfg.APIKey != "env-key" || cfg.Model != "env-model" {
		t.Fatalf("cfg = %#v, want env values", cfg)
	}
	if d.Name() != "openai_compat" {
		t.Fatalf("Name() = %q, want openai_compat with key present", d.Name())
	}
}

func TestDynamicLayersOverrides(t *testing.T) {
	d := NewDynamic(baseConfig(), func(context.Context) Overrides {
		return Overrides{BaseURL: "https://db.example.com/", APIKey: "db-key", ChatModel: "db-model"}
	})
	cfg := d.effectiveConfig(context.Background())
	if cfg.BaseURL != "https://db.example.com" {
		t.Fatalf("BaseURL = %q, want trailing slash trimmed", cfg.BaseURL)
	}
	if cfg.APIKey != "db-key" || cfg.Model != "db-model" {
		t.Fatalf("cfg = %#v, want DB overrides", cfg)
	}
}

func TestDynamicIgnoresAnthropicOverride(t *testing.T) {
	d := NewDynamic(baseConfig(), func(context.Context) Overrides {
		return Overrides{Provider: "anthropic", APIKey: "anthropic-key", ChatModel: "claude-x"}
	})
	cfg := d.effectiveConfig(context.Background())
	if cfg.APIKey != "env-key" || cfg.Model != "env-model" {
		t.Fatalf("cfg = %#v, want anthropic override ignored entirely", cfg)
	}
}

func TestDynamicFallsBackToStubWhenNoKeyAnywhere(t *testing.T) {
	base := baseConfig()
	base.APIKey = ""
	d := NewDynamic(base, func(context.Context) Overrides { return Overrides{} })
	if d.Name() != "stub" {
		t.Fatalf("Name() = %q, want stub without any key", d.Name())
	}
}

func TestDynamicRebuildsOnlyWhenConfigChanges(t *testing.T) {
	current := Overrides{}
	d := NewDynamic(baseConfig(), func(context.Context) Overrides { return current })

	first := d.resolve(context.Background())
	if second := d.resolve(context.Background()); second != first {
		t.Fatalf("client rebuilt though config unchanged")
	}

	current = Overrides{APIKey: "rotated-key"}
	if third := d.resolve(context.Background()); third == first {
		t.Fatalf("client not rebuilt after override change")
	}
}
