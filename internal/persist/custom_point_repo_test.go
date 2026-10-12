package persist

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCustomPointDeltaBoundsAndTotals(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		before                     CustomPointBalance
		delta                      int64
		balance, credited, debited int64
		wantErr                    error
	}{
		{"credit", CustomPointBalance{Balance: 20, TotalCredited: 30, TotalDebited: 10}, 50, 70, 80, 10, nil},
		{"debit to zero", CustomPointBalance{Balance: 20, TotalCredited: 30, TotalDebited: 10}, -20, 0, 30, 30, nil},
		{"insufficient", CustomPointBalance{Balance: 20}, -21, 0, 0, 0, ErrCustomPointInsufficient},
		{"balance overflow", CustomPointBalance{Balance: math.MaxInt64}, 1, 0, 0, 0, ErrCustomPointOverflow},
		{"credited overflow", CustomPointBalance{TotalCredited: math.MaxInt64}, 1, 0, 0, 0, ErrCustomPointOverflow},
		{"debited overflow", CustomPointBalance{Balance: 1, TotalDebited: math.MaxInt64}, -1, 0, 0, 0, ErrCustomPointOverflow},
		{"minimum signed delta", CustomPointBalance{}, math.MinInt64, 0, 0, 0, ErrCustomPointOverflow},
		{"zero", CustomPointBalance{}, 0, 0, 0, 0, ErrCustomPointInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyCustomPointDelta(tc.before, tc.delta)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v want=%v", err, tc.wantErr)
			}
			if err != nil {
				if got != tc.before {
					t.Fatal("failure mutated balance")
				}
				return
			}
			if got.Balance != tc.balance || got.TotalCredited != tc.credited || got.TotalDebited != tc.debited {
				t.Fatalf("unexpected totals: %+v", got)
			}
		})
	}
}

// This opt-in test creates and drops only a unique fixture schema. Production
// tables are never in its search_path. scripts/test_gm_points.ps1 supplies a DSN.
func TestCustomPointPostgresAtomicity(t *testing.T) {
	dsn := os.Getenv("L1JGO_TEST_POINTS_DSN")
	if dsn == "" {
		t.Skip("set L1JGO_TEST_POINTS_DSN or run scripts/test_gm_points.ps1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("codex_gm_points_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("fixture cleanup failed: %v", err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE accounts(name VARCHAR(32) PRIMARY KEY); INSERT INTO accounts VALUES('fixture'),('zero'),('overflow'),('parallel')`); err != nil {
		t.Fatal(err)
	}
	migration, err := migrations.ReadFile("migrations/033_gm_custom_points.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	repo := NewCustomPointRepo(&DB{Pool: pool})
	adjust := func(login string, delta int64) (*CustomPointBalance, error) {
		return repo.AdminAdjust(ctx, login, "MP", delta, "fixture memo", "fixtureGM")
	}
	assertState := func(login string, balance, logCount int64) {
		t.Helper()
		var gotBalance, gotCount int64
		if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT balance FROM custom_points WHERE login=$1 AND currency_code='MP'),0),(SELECT COUNT(*) FROM custom_point_logs WHERE login=$1)`, login).Scan(&gotBalance, &gotCount); err != nil {
			t.Fatal(err)
		}
		if gotBalance != balance || gotCount != logCount {
			t.Fatalf("%s balance/logs=%d/%d want=%d/%d", login, gotBalance, gotCount, balance, logCount)
		}
	}
	if _, err = adjust("fixture", 100); err != nil {
		t.Fatal(err)
	}
	if _, err = adjust("fixture", -30); err != nil {
		t.Fatal(err)
	}
	assertState("fixture", 70, 2)
	for _, tc := range []struct {
		login string
		delta int64
		want  error
	}{
		{"fixture", -71, ErrCustomPointInsufficient}, {"zero", -1, ErrCustomPointInsufficient}, {"missing", 1, ErrCustomPointAccount},
	} {
		if _, err := adjust(tc.login, tc.delta); !errors.Is(err, tc.want) {
			t.Fatalf("%s error=%v want=%v", tc.login, err, tc.want)
		}
	}
	assertState("fixture", 70, 2)
	assertState("zero", 0, 0)
	var emptyRows int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM custom_points WHERE login='zero'`).Scan(&emptyRows); err != nil || emptyRows != 0 {
		t.Fatalf("failed debit left a new row: %d %v", emptyRows, err)
	}
	if _, err = adjust("overflow", math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if _, err = adjust("overflow", 1); !errors.Is(err, ErrCustomPointOverflow) {
		t.Fatalf("overflow accepted: %v", err)
	}
	assertState("overflow", math.MaxInt64, 1)
	// Force an audit insert failure after the UPDATE; the balance must roll back.
	if _, err = pool.Exec(ctx, `ALTER TABLE custom_point_logs ADD CONSTRAINT reject_fixture_test CHECK (memo <> 'force_fail')`); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.AdminAdjust(ctx, "fixture", "MP", 1, "force_fail", "fixtureGM"); err == nil {
		t.Fatal("injected audit failure was accepted")
	}
	assertState("fixture", 70, 2)
	if _, err = pool.Exec(ctx, `UPDATE custom_point_currencies SET enabled=false WHERE code='MP'`); err != nil {
		t.Fatal(err)
	}
	if _, err = adjust("fixture", 1); !errors.Is(err, ErrCustomPointCurrency) {
		t.Fatalf("disabled currency accepted: %v", err)
	}
	assertState("fixture", 70, 2)
	if _, err = pool.Exec(ctx, `UPDATE custom_point_currencies SET enabled=true WHERE code='MP'`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := adjust("parallel", 1)
			if err != nil {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	assertState("parallel", 20, 20)
	query, err := repo.Query(ctx, "parallel", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(query.Balances) != 1 || query.Balances[0].Balance != 20 || len(query.Logs) != 10 {
		t.Fatalf("query result: %+v", query)
	}
	for i, entry := range query.Logs {
		if entry.BalanceAfter != int64(20-i) || entry.GMName != "fixtureGM" || entry.Source != "gm_command" || entry.Type != "admin_adjust" {
			t.Fatalf("audit order or fields: %+v", entry)
		}
	}
	query, err = repo.Query(ctx, "zero", "MP")
	if err != nil || len(query.Balances) != 1 || query.Balances[0].Balance != 0 || len(query.Logs) != 0 {
		t.Fatalf("unused account currency: %+v %v", query, err)
	}
}
