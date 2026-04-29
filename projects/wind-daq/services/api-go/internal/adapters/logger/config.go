package logger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

type Config struct {
	Level    string       `json:"level"`
	Format   string       `json:"format"`
	Console  bool         `json:"console"`
	File     FileConfig   `json:"file"`
	Frontend FrontendConf `json:"frontend"`
}

type FileConfig struct {
	Enabled    bool   `json:"enabled"`
	Path       string `json:"path"`
	MaxSizeMB  int    `json:"maxSizeMB"`
	MaxBackups int    `json:"maxBackups"`
	MaxAgeDays int    `json:"maxAgeDays"`
	Compress   bool   `json:"compress"`
}

type FrontendConf struct {
	Enabled    bool   `json:"enabled"`
	BufferSize int    `json:"bufferSize"`
	MinLevel   string `json:"minLevel"`
}

func DefaultConfig() Config {
	return Config{
		Level:   "info",
		Format:  "text",
		Console: true,
		File: FileConfig{
			Enabled:    true,
			Path:       "logs/wind-daq.log",
			MaxSizeMB:  50,
			MaxBackups: 10,
			MaxAgeDays: 30,
			Compress:   true,
		},
		Frontend: FrontendConf{
			Enabled:    true,
			BufferSize: 500,
			MinLevel:   "warn",
		},
	}
}

func LoadConfig(path string) Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("Failed to parse logging config, using defaults", "path", path, "err", err)
	}
	return cfg
}

func (c Config) SlogLevel() slog.Level {
	switch c.Level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (c Config) FrontendMinLevel() slog.Level {
	switch c.Frontend.MinLevel {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

func (c Config) Validate() error {
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Level] {
		return fmt.Errorf("invalid log level: %s", c.Level)
	}
	if c.Format != "text" && c.Format != "json" {
		return fmt.Errorf("invalid log format: %s (expected text or json)", c.Format)
	}
	return nil
}
