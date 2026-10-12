package persist

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/l1jgo/server/internal/world"
)

// NormalizeIPAddress 去掉 TCP 埠號，統一 IPv4、IPv6 與 IPv4-mapped IPv6，防止換埠繞過封鎖。
func NormalizeIPAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	ip, err := netip.ParseAddr(value)
	if err != nil {
		return "", fmt.Errorf("invalid IP address: %w", err)
	}
	return ip.Unmap().WithZone("").String(), nil
}

func (r *AccountRepo) SetBanned(ctx context.Context, name string, banned bool) error {
	result, err := r.db.Pool.Exec(ctx, `UPDATE accounts SET banned=$2 WHERE name=$1`, name, banned)
	if err == nil && result.RowsAffected() == 0 {
		return fmt.Errorf("account not found: %s", name)
	}
	return err
}

func (r *AccountRepo) IsIPBanned(ctx context.Context, address string) (bool, error) {
	ip, err := NormalizeIPAddress(address)
	if err != nil {
		return false, err
	}
	var banned bool
	err = r.db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM banned_ips WHERE ip=$1::inet)`, ip).Scan(&banned)
	return banned, err
}

func (r *AccountRepo) SetIPBanned(ctx context.Context, address string, banned bool, actor string) error {
	ip, err := NormalizeIPAddress(address)
	if err != nil {
		return err
	}
	if banned {
		_, err = r.db.Pool.Exec(ctx, `INSERT INTO banned_ips(ip, banned_by) VALUES ($1::inet,$2) ON CONFLICT (ip) DO NOTHING`, ip, actor)
	} else {
		_, err = r.db.Pool.Exec(ctx, `DELETE FROM banned_ips WHERE ip=$1::inet`, ip)
	}
	return err
}

type GMGiftRequest struct {
	Account                     string
	ByLevel                     bool
	MinLevel, MaxLevel          int16
	ItemID, Count               int32
	Enchant, Bless, ChargeCount int16
	Stackable                   bool
}

// GiveGMGift 對每個符合條件的帳號僅發放一次（Java L1DwarfInventory.present）。
// 以單一交易發送到個人倉庫；不可堆疊物品逐件建立，失敗時全部回滾。
func (r *WarehouseRepo) GiveGMGift(ctx context.Context, gift GMGiftRequest) (int, error) {
	if gift.ItemID <= 0 || gift.Count <= 0 || gift.Enchant < -127 || gift.Enchant > 127 {
		return 0, fmt.Errorf("invalid gift item, count or enchant")
	}
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	query := `SELECT name FROM accounts WHERE ($1='*' OR name=$1) ORDER BY name`
	params := []any{gift.Account}
	if gift.ByLevel {
		if gift.MinLevel < 1 || gift.MaxLevel < gift.MinLevel {
			return 0, fmt.Errorf("invalid gift level range")
		}
		query = `SELECT DISTINCT account_name FROM characters WHERE level BETWEEN $1 AND $2 AND deleted_at IS NULL ORDER BY account_name`
		params = []any{gift.MinLevel, gift.MaxLevel}
	}
	rows, err := tx.Query(ctx, query, params...)
	if err != nil {
		return 0, err
	}
	var accounts []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return 0, err
		}
		accounts = append(accounts, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, account := range accounts {
		instances, count := gift.Count, int32(1)
		if gift.Stackable {
			instances, count = 1, gift.Count
		}
		for i := int32(0); i < instances; i++ {
			_, err := tx.Exec(ctx, `INSERT INTO warehouse_items(account_name,char_name,wh_type,item_id,count,enchant_lvl,bless,identified,item_obj_id,charge_count) VALUES ($1,'',3,$2,$3,$4,$5,FALSE,$6,$7)`, account, gift.ItemID, count, gift.Enchant, gift.Bless, world.NextItemObjID(), gift.ChargeCount)
			if err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(accounts), nil
}

// ProcessHomeTown 實作 Java TownTable 日結與 HomeTownTimeController 月結。
// 城鎮欄位獨立於一般角色存檔，結算不會被在線角色的舊快照覆寫。
func (r *CharacterRepo) ProcessHomeTown(ctx context.Context, monthly bool) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if !monthly {
		_, err = tx.Exec(ctx, `UPDATE towns SET tax_rate=tax_rate_reserved, sales_money_yesterday=sales_money, sales_money=0`)
	} else {
		// 鎖定城鎮及參與結算的角色，確保分紅、選舉與歸零為同一原子操作。
		_, err = tx.Exec(ctx, `LOCK TABLE towns, characters IN SHARE ROW EXCLUSIVE MODE`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE towns t SET leader_id=COALESCE((SELECT id FROM characters c WHERE c.home_town_id=t.town_id AND c.deleted_at IS NULL ORDER BY contribution DESC,id LIMIT 1),0), leader_name=COALESCE((SELECT name FROM characters c WHERE c.home_town_id=t.town_id AND c.deleted_at IS NULL ORDER BY contribution DESC,id LIMIT 1),'')`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `WITH rates AS (SELECT t.town_id, CASE WHEN COALESCE(SUM(c.contribution),0)>0 THEN FLOOR(t.town_fix_tax::numeric/SUM(c.contribution)*100)/100 ELSE 0 END AS unit FROM towns t LEFT JOIN characters c ON c.home_town_id=t.town_id AND c.deleted_at IS NULL GROUP BY t.town_id,t.town_fix_tax) UPDATE characters c SET town_pay=FLOOR(c.contribution*r.unit)::bigint FROM rates r WHERE c.home_town_id=r.town_id AND c.deleted_at IS NULL`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE towns SET tax_rate=0,tax_rate_reserved=0,sales_money_yesterday=sales_money,sales_money=0,town_tax=0,town_fix_tax=0`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE characters SET contribution=0,home_town_id=CASE WHEN home_town_id=-1 THEN 0 ELSE home_town_id END`)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// HomeTownLeaderNotices 回傳有村長之居民，用於月結後的 S_PacketBox.MSG_TOWN_LEADER。
func (r *CharacterRepo) HomeTownLeaderNotices(ctx context.Context) (map[int32]string, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT c.id,t.leader_name FROM characters c JOIN towns t ON t.town_id=c.home_town_id WHERE c.deleted_at IS NULL AND t.leader_id<>0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notices := make(map[int32]string)
	for rows.Next() {
		var id int32
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		notices[id] = name
	}
	return notices, rows.Err()
}
