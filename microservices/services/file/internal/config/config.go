// Package config provides 12-factor, environment-only configuration for the
// file service. It intentionally keeps the same struct shape, global Cfg
// variable, helper methods, and environment variable names as the monolith's
// config package so that code copied from the monolith keeps working
// unchanged and docker-compose environments stay uniform.
package config

var Cfg Config

// Load fills the package-level Cfg from environment variables layered over
// Defaults. Env var names match the monolith exactly.
func Load() error {
	cfg := Defaults()
	applyEnv(&cfg)
	if err := validate(cfg); err != nil {
		return err
	}
	Cfg = cfg
	return nil
}
