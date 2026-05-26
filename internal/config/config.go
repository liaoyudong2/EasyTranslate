package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

const (
	DefaultPort = 8080
	AppName     = "lan-transfer"
)

type Config struct {
	Port         int    `json:"port"`
	StorageDir   string `json:"storageDir"`
	AccessCode   string `json:"accessCode"`
	AutoStart    bool   `json:"autoStart"`
	RememberCode bool   `json:"rememberCode"`
}

func DefaultStorageDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Documents", "简传", "uploads")
	}
	return filepath.Join(".", "data", "uploads")
}

func IsLegacyRootStorageDir(dir string) bool {
	clean := filepath.Clean(dir)
	return clean == filepath.Join(string(filepath.Separator), "data", "uploads")
}

func Default() Config {
	return Config{
		Port:       DefaultPort,
		StorageDir: DefaultStorageDir(),
		AccessCode: MustAccessCode(),
	}
}

func Normalize(cfg Config) Config {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = DefaultPort
	}
	if cfg.StorageDir == "" {
		cfg.StorageDir = DefaultStorageDir()
	}
	if cfg.AccessCode == "" {
		cfg.AccessCode = MustAccessCode()
	}
	return cfg
}

func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AppName, "config.json"), nil
}

func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Default(), err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}
	if !cfg.RememberCode {
		cfg.AccessCode = MustAccessCode()
	}
	return Normalize(cfg), nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	saveCfg := Normalize(cfg)
	if !saveCfg.RememberCode {
		saveCfg.AccessCode = ""
	}
	data, err := json.MarshalIndent(saveCfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func MustAccessCode() string {
	code, err := GenerateAccessCode(6)
	if err != nil {
		return "000000"
	}
	return code
}

func GenerateAccessCode(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("访问码长度必须大于 0")
	}
	const digits = "0123456789"
	result := make([]byte, length)
	max := big.NewInt(int64(len(digits)))
	for i := range result {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		result[i] = digits[n.Int64()]
	}
	return string(result), nil
}
