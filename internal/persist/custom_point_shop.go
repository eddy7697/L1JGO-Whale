package persist

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/l1jgo/server/internal/data"
)

type CustomPointShopRepo struct{ db *DB }

func NewCustomPointShopRepo(db *DB) *CustomPointShopRepo { return &CustomPointShopRepo{db: db} }

// Load reads the two catalogue tables from one consistent snapshot.
func (r *CustomPointShopRepo) Load(ctx context.Context) ([]data.CustomPointShop, []data.CustomPointShopItem, error) {
	tx, err := r.db.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT shop_id, npc_id, name, enabled FROM custom_point_shops`)
	if err != nil {
		return nil, nil, err
	}
	var shops []data.CustomPointShop
	for rows.Next() {
		var shop data.CustomPointShop
		if err := rows.Scan(&shop.ID, &shop.NpcID, &shop.Name, &shop.Enabled); err != nil {
			rows.Close()
			return nil, nil, err
		}
		shops = append(shops, shop)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows, err = tx.Query(ctx, `SELECT id, shop_id, item_id, price_currency, price, pack_count, daily_limit, account_limit, sort_order, enabled FROM custom_point_shop_items WHERE enabled = TRUE`)
	if err != nil {
		return nil, nil, err
	}
	var items []data.CustomPointShopItem
	for rows.Next() {
		var item data.CustomPointShopItem
		if err := rows.Scan(&item.ID, &item.ShopID, &item.ItemID, &item.Currency, &item.Price, &item.PackCount, &item.DailyLimit, &item.AccountLimit, &item.SortOrder, &item.Enabled); err != nil {
			rows.Close()
			return nil, nil, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return shops, items, nil
}
