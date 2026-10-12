package persist

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	ErrCustomPointAccount      = errors.New("找不到帳號")
	ErrCustomPointCurrency     = errors.New("幣別不存在或已停用")
	ErrCustomPointInsufficient = errors.New("點數餘額不足")
	ErrCustomPointOverflow     = errors.New("點數或累計異動超過 64 位元整數上限")
	ErrCustomPointInput        = errors.New("點數參數不合法")
)

type CustomPointBalance struct {
	Currency      string
	DisplayName   string
	Balance       int64
	TotalCredited int64
	TotalDebited  int64
}

type CustomPointLog struct {
	ID              int64
	Currency        string
	Delta           int64
	BalanceAfter    int64
	Type            string
	Source          string
	ExternalOrderID string
	Memo            string
	GMName          string
	CreatedAt       time.Time
}

type CustomPointQuery struct {
	Balances []CustomPointBalance
	Logs     []CustomPointLog
}

type CustomPointRepo struct{ db *DB }

func NewCustomPointRepo(db *DB) *CustomPointRepo { return &CustomPointRepo{db: db} }

// ValidateCustomPointAdjustment also protects callers outside the GM handler.
func ValidateCustomPointAdjustment(login, currency string, delta int64, memo string) error {
	if !validCustomPointText(login, 32) || !validCustomPointText(currency, 16) || !validCustomPointText(memo, 255) || delta == 0 {
		return ErrCustomPointInput
	}
	if delta == math.MinInt64 {
		return ErrCustomPointOverflow // Its absolute value cannot fit in total_debited.
	}
	return nil
}

func validCustomPointText(value string, limit int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= limit
}

// applyCustomPointDelta never mutates its input on failure.
func applyCustomPointDelta(current CustomPointBalance, delta int64) (CustomPointBalance, error) {
	if delta == 0 || current.Balance < 0 || current.TotalCredited < 0 || current.TotalDebited < 0 {
		return current, ErrCustomPointInput
	}
	if delta == math.MinInt64 {
		return current, ErrCustomPointOverflow
	}
	if delta > 0 {
		if current.Balance > math.MaxInt64-delta || current.TotalCredited > math.MaxInt64-delta {
			return current, ErrCustomPointOverflow
		}
		current.Balance += delta
		current.TotalCredited += delta
	} else {
		amount := -delta
		if current.Balance < amount {
			return current, ErrCustomPointInsufficient
		}
		if current.TotalDebited > math.MaxInt64-amount {
			return current, ErrCustomPointOverflow
		}
		current.Balance -= amount
		current.TotalDebited += amount
	}
	return current, nil
}

// AdminAdjust commits balance, cumulative totals and the audit record together.
// Row locks serialize administrators and future shop/API writers across servers.
func (r *CustomPointRepo) AdminAdjust(ctx context.Context, login, currency string, delta int64, memo, gmName string) (*CustomPointBalance, error) {
	login, currency = strings.ToLower(strings.TrimSpace(login)), strings.ToUpper(strings.TrimSpace(currency))
	memo = strings.TrimSpace(memo)
	if err := ValidateCustomPointAdjustment(login, currency, delta, memo); err != nil {
		return nil, err
	}
	if !validCustomPointText(gmName, 32) {
		return nil, ErrCustomPointInput
	}
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	var found string
	if err = tx.QueryRow(ctx, `SELECT name FROM accounts WHERE name=$1 FOR KEY SHARE`, login).Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCustomPointAccount
		}
		return nil, err
	}
	current := CustomPointBalance{Currency: currency}
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT display_name, enabled FROM custom_point_currencies WHERE code=$1 FOR SHARE`, currency).Scan(&current.DisplayName, &enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCustomPointCurrency
		}
		return nil, err
	}
	if !enabled {
		return nil, ErrCustomPointCurrency
	}
	if _, err = tx.Exec(ctx, `INSERT INTO custom_points(login,currency_code) VALUES($1,$2) ON CONFLICT DO NOTHING`, login, currency); err != nil {
		return nil, err
	}
	if err = tx.QueryRow(ctx, `SELECT balance,total_credited,total_debited FROM custom_points WHERE login=$1 AND currency_code=$2 FOR UPDATE`, login, currency).Scan(&current.Balance, &current.TotalCredited, &current.TotalDebited); err != nil {
		return nil, err
	}
	next, err := applyCustomPointDelta(current, delta)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE custom_points SET balance=$3,total_credited=$4,total_debited=$5,updated_at=NOW() WHERE login=$1 AND currency_code=$2`, login, currency, next.Balance, next.TotalCredited, next.TotalDebited); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO custom_point_logs(login,currency_code,delta,balance_after,type,source,memo,gm_name) VALUES($1,$2,$3,$4,'admin_adjust','gm_command',$5,$6)`, login, currency, delta, next.Balance, memo, gmName); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &next, nil
}

// Query returns balances and the newest ten audit records from one snapshot.
// An omitted currency means all balances; an unused named currency reports zero.
func (r *CustomPointRepo) Query(ctx context.Context, login, currency string) (*CustomPointQuery, error) {
	login, currency = strings.ToLower(strings.TrimSpace(login)), strings.ToUpper(strings.TrimSpace(currency))
	if !validCustomPointText(login, 32) || (currency != "" && !validCustomPointText(currency, 16)) {
		return nil, ErrCustomPointInput
	}
	tx, err := r.db.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	result := &CustomPointQuery{}
	rows, err := tx.Query(ctx, `SELECT p.currency_code,c.display_name,p.balance,p.total_credited,p.total_debited FROM custom_points p JOIN custom_point_currencies c ON c.code=p.currency_code WHERE p.login=$1 AND ($2='' OR p.currency_code=$2) ORDER BY c.sort_order,p.currency_code`, login, currency)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var balance CustomPointBalance
		if err = rows.Scan(&balance.Currency, &balance.DisplayName, &balance.Balance, &balance.TotalCredited, &balance.TotalDebited); err != nil {
			rows.Close()
			return nil, err
		}
		result.Balances = append(result.Balances, balance)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if currency != "" && len(result.Balances) == 0 {
		balance := CustomPointBalance{Currency: currency, DisplayName: currency}
		err = tx.QueryRow(ctx, `SELECT display_name FROM custom_point_currencies WHERE code=$1`, currency).Scan(&balance.DisplayName)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		result.Balances = append(result.Balances, balance)
	}
	rows, err = tx.Query(ctx, `SELECT id,currency_code,delta,balance_after,type,source,COALESCE(external_order_id,''),memo,gm_name,created_at FROM custom_point_logs WHERE login=$1 AND ($2='' OR currency_code=$2) ORDER BY id DESC LIMIT 10`, login, currency)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var entry CustomPointLog
		if err = rows.Scan(&entry.ID, &entry.Currency, &entry.Delta, &entry.BalanceAfter, &entry.Type, &entry.Source, &entry.ExternalOrderID, &entry.Memo, &entry.GMName, &entry.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		result.Logs = append(result.Logs, entry)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("query custom point logs: %w", err)
	}
	rows.Close()
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
