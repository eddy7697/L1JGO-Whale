package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWeatherIsOptInInDefaultsAndShippedConfigs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.toml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{path, "../../config/server.toml", "../../config/server.docker.toml"} {
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.World.WeatherEnabled || cfg.World.WeatherInterval <= 0 {
			t.Fatalf("%s 應預設停用自動天氣，且保留有效的選用間隔: %+v", path, cfg.World)
		}
	}
}
