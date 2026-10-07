package qualitycheck

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/shanejonas/cyclo/domain/quality"
)

func LoadConfig(path string) (quality.Config, error) {
	config := quality.DefaultConfig()
	if path == "" {
		return config, nil
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("read quality config: %w", err)
	}
	// Decode into a config without prefixes so user prefixes extend the
	// defaults instead of replacing them.
	defaults := config.Prefixes
	config.Prefixes = nil
	if err := decodeConfig(source, &config); err != nil {
		return config, err
	}
	config.Prefixes = append(defaults, config.Prefixes...)
	return config, config.Validate()
}

// decodeConfig parses TOML source into config, rejecting unknown keys.
func decodeConfig(source []byte, config *quality.Config) error {
	metadata, err := toml.Decode(string(source), config)
	if err != nil {
		return fmt.Errorf("decode quality config: %w", err)
	}
	if keys := metadata.Undecoded(); len(keys) > 0 {
		names := make([]string, len(keys))
		for index, key := range keys {
			names[index] = key.String()
		}
		return fmt.Errorf("unknown quality config keys: %s", strings.Join(names, ", "))
	}
	return nil
}
