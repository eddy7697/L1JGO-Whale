package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaxOnlineUsersConfig(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       int
		fail       bool
	}{
		{"default", "", 30, false},
		{"configured", "[server]\nmax_online_users = 120\n", 120, false},
		{"privileged-only", "[server]\nmax_online_users = 0\n", 0, false},
		{"negative", "[server]\nmax_online_users = -1\n", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.toml")
			if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tc.fail {
				if err == nil {
					t.Fatal("expected invalid limit rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server.MaxOnlineUsers != tc.want {
				t.Fatalf("limit = %d, want %d", cfg.Server.MaxOnlineUsers, tc.want)
			}
		})
	}
}
