package persist

import (
	"context"
	"time"
)

// LastActiveByNames 讓 GM 巡查以一次查詢取得登入時間，不讀取帳號密碼欄位。
func (r *AccountRepo) LastActiveByNames(ctx context.Context, names []string) (map[string]time.Time, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT name, last_active FROM accounts WHERE name=ANY($1) AND last_active IS NOT NULL`, names)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]time.Time)
	for rows.Next() {
		var name string
		var active time.Time
		if err := rows.Scan(&name, &active); err != nil {
			return nil, err
		}
		result[name] = active
	}
	return result, rows.Err()
}
