package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabasePath      string
	SessionTTL        time.Duration
	WorkerPoll        time.Duration
	WorkerLease       time.Duration
	ShutdownTimeout   time.Duration
	BootstrapEmail    string
	BootstrapPassword string
	WorkerEnabled     bool
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          value("HTTP_ADDR", ":8080"),
		DatabasePath:      value("DATABASE_PATH", "./data/launchpad.db"),
		BootstrapEmail:    strings.TrimSpace(os.Getenv("BOOTSTRAP_OPERATOR_EMAIL")),
		BootstrapPassword: os.Getenv("BOOTSTRAP_OPERATOR_PASSWORD"),
		WorkerEnabled:     boolValue("WORKER_ENABLED", true),
	}
	var err error
	if cfg.SessionTTL, err = duration("SESSION_TTL", 12*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.WorkerPoll, err = duration("WORKER_POLL_INTERVAL", 250*time.Millisecond); err != nil {
		return Config{}, err
	}
	if cfg.WorkerLease, err = duration("WORKER_LEASE_DURATION", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = duration("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPAddr == "" {
		return Config{}, errors.New("HTTP_ADDR must not be empty")
	}
	if cfg.DatabasePath == "" {
		return Config{}, errors.New("DATABASE_PATH must not be empty")
	}
	if cfg.SessionTTL < time.Minute {
		return Config{}, errors.New("SESSION_TTL must be at least one minute")
	}
	if cfg.WorkerLease <= cfg.WorkerPoll {
		return Config{}, errors.New("WORKER_LEASE_DURATION must exceed WORKER_POLL_INTERVAL")
	}
	if (cfg.BootstrapEmail == "") != (cfg.BootstrapPassword == "") {
		return Config{}, errors.New("bootstrap email and password must be configured together")
	}
	if cfg.BootstrapPassword != "" && len(cfg.BootstrapPassword) < 12 {
		return Config{}, errors.New("bootstrap password must contain at least 12 characters")
	}
	return cfg, nil
}

func value(name, fallback string) string {
	if raw, ok := os.LookupEnv(name); ok {
		return strings.TrimSpace(raw)
	}
	return fallback
}

func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw := value(name, fallback.String())
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return parsed, nil
}

func boolValue(name string, fallback bool) bool {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return parsed
}
