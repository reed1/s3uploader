package server

import (
	"os"
	"regexp"

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

var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

func substituteEnvVars(data []byte) []byte {
	return envVarRegex.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := envVarRegex.FindSubmatch(match)[1]
		if val, ok := os.LookupEnv(string(varName)); ok {
			return []byte(val)
		}
		return match
	})
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	data = substituteEnvVars(data)

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

	data = substituteEnvVars(data)

	var cf clientsFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, err
	}

	return cf.Clients, nil
}
