package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultRuntimeConfigPath = "config/config.json"

	defaultMapping     = "[]\n"
	defaultCredentials = `# Generated/updated by knx-tuya-gw. Keep this file private.
TUYA_PROVISION_CLIENT_ID=
TUYA_PRODUCT_ID=
TUYA_DEVICE_ID=
TUYA_DEVICE_SECRET=
TUYA_MQTT_BROKER=
`
)

// BootstrapRuntime creates editable templates for missing runtime files.
// The caller should exit when files are created so an operator can review them.
func BootstrapRuntime(configPath string) ([]string, error) {
	created := make([]string, 0, 3)
	if _, err := os.Stat(configPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect runtime config %s: %w", configPath, err)
		}
		cfg := Default()
		cfg.TuyaMQTT.APIKey = "replace_with_tuya_api_key"
		if err := writeJSONFile(configPath, cfg, 0o600); err != nil {
			return nil, fmt.Errorf("create default runtime config %s: %w", configPath, err)
		}
		created = append(created, configPath)
	}

	cfg, err := Load(configPath)
	if err != nil {
		return created, err
	}
	if made, err := ensureFile(cfg.MappingFile, []byte(defaultMapping), 0o644); err != nil {
		return created, fmt.Errorf("create default KNX mapping %s: %w", cfg.MappingFile, err)
	} else if made {
		created = append(created, cfg.MappingFile)
	}
	if cfg.TuyaMQTT.Enabled {
		if made, err := ensureFile(
			cfg.TuyaMQTT.CredentialsFile,
			[]byte(defaultCredentials),
			0o600,
		); err != nil {
			return created, fmt.Errorf(
				"create default Tuya credentials %s: %w",
				cfg.TuyaMQTT.CredentialsFile,
				err,
			)
		} else if made {
			created = append(created, cfg.TuyaMQTT.CredentialsFile)
		}
	}
	if cfg.TuyaMQTT.CAFile != "" {
		if _, err := os.Stat(cfg.TuyaMQTT.CAFile); err != nil {
			if os.IsNotExist(err) {
				return created, fmt.Errorf(
					"tuya_mqtt.ca_file does not exist: %s; add the certificate or clear ca_file",
					cfg.TuyaMQTT.CAFile,
				)
			}
			return created, fmt.Errorf("inspect Tuya CA file %s: %w", cfg.TuyaMQTT.CAFile, err)
		}
	}
	return created, nil
}

func writeJSONFile(path string, value interface{}, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeNewFile(path, data, mode)
}

func ensureFile(path string, data []byte, mode os.FileMode) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return true, writeNewFile(path, data, mode)
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}
