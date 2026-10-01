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
	defaults := config.Prefixes
	config.Prefixes = nil
	metadata, err := toml.Decode(string(source), &config)
	if err != nil {
		return config, fmt.Errorf("decode quality config: %w", err)
	}
	if keys := metadata.Undecoded(); len(keys) > 0 {
		names := make([]string, len(keys))
		for index, key := range keys {
			names[index] = key.String()
		}
		return config, fmt.Errorf("unknown quality config keys: %s", strings.Join(names, ", "))
	}
	config.Prefixes = append(defaults, config.Prefixes...)
	return config, config.Validate()
}
