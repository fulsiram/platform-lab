package appconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

func LoadUserConfig(path string) (Config, string, error) {
	configPath, explicit := ResolveConfigPath(path)
	cfg, found, err := loadFile(configPath)
	if err != nil {
		return Config{}, "", err
	}
	if !found && explicit {
		return Config{}, configPath, nil
	}
	return cfg, configPath, nil
}

func SaveUserConfig(path string, cfg Config) error {
	if path == "" {
		return errors.New("config path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file %q: %w", path, err)
	}
	return nil
}
