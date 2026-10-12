package persist

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/l1jgo/server/internal/config"
)

func TestDatabaseConfigConnectionSettings(t *testing.T) {
	// pgx reads these settings from the process environment. Isolate the test
	// from local service files, SSL settings, and other developer overrides.
	for _, name := range []string{
		"PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD",
		"PGPASSFILE", "PGAPPNAME", "PGCONNECT_TIMEOUT", "PGSSLMODE",
		"PGSSLKEY", "PGSSLCERT", "PGSSLSNI", "PGSSLROOTCERT", "PGSSLPASSWORD",
		"PGTARGETSESSIONATTRS", "PGSERVICE", "PGSERVICEFILE",
	} {
		t.Setenv(name, "")
	}

	type connectionSettings struct {
		host     string
		port     uint16
		user     string
		password string
		database string
	}
	env := connectionSettings{
		host:     "environment-db.invalid",
		port:     55432,
		user:     "environment user",
		password: `test $ @ : / # spaces ' " \ end`,
		database: "environment database",
	}
	t.Setenv("PGHOST", env.host)
	t.Setenv("PGPORT", strconv.Itoa(int(env.port)))
	t.Setenv("PGUSER", env.user)
	t.Setenv("PGPASSWORD", env.password)
	t.Setenv("PGDATABASE", env.database)

	writeConfig := func(name, contents string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// An explicitly selected local TOML must still be able to override the
	// environment, including credentials with URI-sensitive characters.
	local := connectionSettings{
		host:     "local-db.invalid",
		port:     55433,
		user:     "local user",
		password: `local $ @ : / # spaces ' " \ end`,
		database: "local database",
	}
	localDSN := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(local.user, local.password),
		Host:     local.host + ":" + strconv.Itoa(int(local.port)),
		Path:     "/" + local.database,
		RawQuery: "sslmode=disable",
	}).String()
	localPath := writeConfig("local.toml", "[database]\ndsn = "+strconv.Quote(localDSN)+"\n")

	for _, tc := range []struct {
		name string
		path string
		want connectionSettings
	}{
		{"docker TOML uses PG environment", filepath.Join("..", "..", "config", "server.docker.toml"), env},
		{"server TOML uses PG environment", filepath.Join("..", "..", "config", "server.toml"), env},
		{"defaults use PG environment", writeConfig("defaults.toml", ""), env},
		{"explicit local DSN overrides PG environment", localPath, local},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Load(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			// Use the same parsing entry point as NewDB without opening a connection.
			poolCfg, err := pgxpool.ParseConfig(cfg.Database.DSN)
			if err != nil {
				t.Fatal(err)
			}
			got := poolCfg.ConnConfig
			for _, field := range []struct {
				name string
				got  string
				want string
			}{
				{"host", got.Host, tc.want.host},
				{"port", strconv.Itoa(int(got.Port)), strconv.Itoa(int(tc.want.port))},
				{"user", got.User, tc.want.user},
				{"password", got.Password, tc.want.password},
				{"database", got.Database, tc.want.database},
			} {
				if field.got != field.want {
					t.Errorf("%s did not preserve the expected connection setting", field.name)
				}
			}
			if got.TLSConfig != nil {
				t.Error("development DSN should disable SSL")
			}
		})
	}
}
