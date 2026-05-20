package config

import (
	"testing"
	"time"
)

func TestDatabaseConfigConnectionPoolLifetimeDefaults(t *testing.T) {
	cfg := DatabaseConfig{}

	if got := cfg.EffectiveConnMaxLifetime(); got != 5*time.Minute {
		t.Fatalf("conn max lifetime = %s, want 5m", got)
	}
	if got := cfg.EffectiveConnMaxIdleTime(); got != 3*time.Minute {
		t.Fatalf("conn max idle time = %s, want 3m", got)
	}
}

func TestDatabaseConfigConnectionPoolLifetimeOverrides(t *testing.T) {
	cfg := DatabaseConfig{
		ConnMaxLifetimeSeconds: 120,
		ConnMaxIdleTimeSeconds: 45,
	}

	if got := cfg.EffectiveConnMaxLifetime(); got != 2*time.Minute {
		t.Fatalf("conn max lifetime = %s, want 2m", got)
	}
	if got := cfg.EffectiveConnMaxIdleTime(); got != 45*time.Second {
		t.Fatalf("conn max idle time = %s, want 45s", got)
	}
}
