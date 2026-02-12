package client

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server          ServerConfig    `yaml:"server"`
	Database        DatabaseConfig  `yaml:"database"`
	Watches         []WatchConfig   `yaml:"watches"`
	Scan            ScanConfig      `yaml:"scan"`
	Stability       StabilityConfig `yaml:"stability"`
	Upload          UploadConfig    `yaml:"upload"`
	ExcludePatterns []string        `yaml:"exclude_patterns"`

	excludeRegexps []*regexp.Regexp
}

type ServerConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type WatchConfig struct {
	LocalPath    string `yaml:"local_path"`
	RemotePrefix string `yaml:"remote_prefix"`
}

type ScanConfig struct {
	UploadExisting bool `yaml:"upload_existing"`
}

type StabilityConfig struct {
	DebounceSeconds int `yaml:"debounce_seconds"`
	MaxAttempts     int `yaml:"max_attempts"`
}

type UploadConfig struct {
	RetryAttempts     int `yaml:"retry_attempts"`
	RetryDelaySeconds int `yaml:"retry_delay_seconds"`
	MaxFileSizeMB     int `yaml:"max_file_size_mb"`
}

func expandTilde(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	home, _ := os.UserHomeDir()
	cfg.Database.Path = expandTilde(cfg.Database.Path, home)
	for i := range cfg.Watches {
		cfg.Watches[i].LocalPath = expandTilde(cfg.Watches[i].LocalPath, home)
	}

	if !filepath.IsAbs(cfg.Database.Path) {
		return nil, fmt.Errorf("database.path must be an absolute path, got %q", cfg.Database.Path)
	}
	for _, w := range cfg.Watches {
		if !filepath.IsAbs(w.LocalPath) {
			return nil, fmt.Errorf("watches.local_path must be an absolute path, got %q", w.LocalPath)
		}
	}

	if cfg.Stability.DebounceSeconds == 0 {
		cfg.Stability.DebounceSeconds = 3
	}
	if cfg.Stability.MaxAttempts == 0 {
		cfg.Stability.MaxAttempts = 100
	}
	if cfg.Upload.RetryAttempts == 0 {
		cfg.Upload.RetryAttempts = 3
	}
	if cfg.Upload.RetryDelaySeconds == 0 {
		cfg.Upload.RetryDelaySeconds = 5
	}
	if cfg.Upload.MaxFileSizeMB == 0 {
		cfg.Upload.MaxFileSizeMB = 100
	}

	if err := cfg.CompileExcludePatterns(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) CompileExcludePatterns() error {
	c.excludeRegexps = nil
	for _, pattern := range c.ExcludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid exclude_pattern %q: %w", pattern, err)
		}
		c.excludeRegexps = append(c.excludeRegexps, re)
	}
	return nil
}

func (c *Config) IsExcluded(path string) bool {
	for _, re := range c.excludeRegexps {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
