package client

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Watches   []WatchConfig   `yaml:"watches"`
	Scan      ScanConfig      `yaml:"scan"`
	Stability StabilityConfig `yaml:"stability"`
	Upload    UploadConfig    `yaml:"upload"`
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
	Recursive    bool   `yaml:"recursive"`
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

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
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

	return &cfg, nil
}
