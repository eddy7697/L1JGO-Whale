package persist

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrAccountUnavailable = errors.New("account was removed or banned during login")

// BeginOnlineState holds a database/schema-scoped lease for the server lifetime.
// Only its owner may reset crash leftovers (Java Account.InitialOnlineStatus).
// A dedicated connection keeps even a one-connection application pool usable.
// Call before accepting clients; close the lease after disconnect cleanup.
func (r *AccountRepo) BeginOnlineState(ctx context.Context) (*pgx.Conn, error) {
	conn, err := pgx.ConnectConfig(ctx, r.db.Pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close(context.Background())
		}
	}()
	var locked bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended(current_schema() || ':l1jgo:account-online', 0))`).Scan(&locked)
	if err != nil {
		return nil, fmt.Errorf("lock account online state: %w", err)
	}
	if !locked {
		return nil, errors.New("another game server owns this database's account online state")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `UPDATE accounts SET online = false WHERE online`); err != nil {
		return nil, fmt.Errorf("reset account online state: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE characters SET online_status = false WHERE online_status`); err != nil {
		return nil, fmt.Errorf("reset character online state: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	ok = true
	return conn, nil
}

// TryMarkOnline atomically claims an offline account. The process-local owner
// is registered only after this succeeds; database errors must reject login.
func (r *AccountRepo) TryMarkOnline(ctx context.Context, name string) (bool, error) {
	result, err := r.db.Pool.Exec(ctx,
		`UPDATE accounts SET online = true WHERE name = $1 AND NOT online AND NOT banned`, name)
	if err != nil {
		return false, err
	}
	if result.RowsAffected() == 1 {
		return true, nil
	}
	var banned bool
	if err := r.db.Pool.QueryRow(ctx, `SELECT banned FROM accounts WHERE name = $1`, name).Scan(&banned); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrAccountUnavailable
		}
		return false, err
	}
	if banned {
		return false, ErrAccountUnavailable
	}
	return false, nil
}
