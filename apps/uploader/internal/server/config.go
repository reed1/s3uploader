package server

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig   `yaml:"server"`
	S3            S3Config       `yaml:"s3"`
	Database      DatabaseConfig `yaml:"database"`
	ClientsConfig string         `yaml:"clients_config"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type S3Config struct {
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	PathPrefix      string `yaml:"path_prefix"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
}

type ClientEntry struct {
	ID     string `yaml:"id"`
	APIKey string `yaml:"api_key"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
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

	return &cfg, nil
}

type clientsFile struct {
	Clients []ClientEntry `yaml:"clients"`
}

func LoadClientsConfig(path string) ([]ClientEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cf clientsFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, err
	}

	seen := make(map[string]string, len(cf.Clients))
	for _, c := range cf.Clients {
		if prev, ok := seen[c.APIKey]; ok {
			return nil, fmt.Errorf("duplicate api_key between clients %q and %q", prev, c.ID)
		}
		seen[c.APIKey] = c.ID
	}

	return cf.Clients, nil
}
