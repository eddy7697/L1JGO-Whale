package persist

import (
	"context"
	"fmt"

	"github.com/l1jgo/server/internal/data"
)

type AdvancedPolymorphRepo struct{ db *DB }

func NewAdvancedPolymorphRepo(db *DB) *AdvancedPolymorphRepo { return &AdvancedPolymorphRepo{db: db} }

// Unlock 對齊 Java：首次建立收錄，之後每次將 unlock_count 加一，不更動預設變身。
func (r *AdvancedPolymorphRepo) Unlock(ctx context.Context, charID, polyID int32) (int32, error) {
	if _, ok := data.AdvancedPolymorphCard(polyID); !ok || charID <= 0 {
		return 0, fmt.Errorf("invalid character or advanced polymorph")
	}
	var count int32
	err := r.db.Pool.QueryRow(ctx, `INSERT INTO character_advanced_polymorphs(char_id,polyid,unlock_count,is_default,unlocked_at,updated_at) VALUES($1,$2,1,FALSE,NOW(),NOW()) ON CONFLICT(char_id,polyid) DO UPDATE SET unlock_count=character_advanced_polymorphs.unlock_count+1,updated_at=NOW() RETURNING unlock_count`, charID, polyID).Scan(&count)
	return count, err
}

// Reset 只刪除指定角色全部收錄；不解除玩家當前變身。
func (r *AdvancedPolymorphRepo) Reset(ctx context.Context, charID int32) error {
	if charID <= 0 {
		return fmt.Errorf("invalid character")
	}
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM character_advanced_polymorphs WHERE char_id=$1`, charID)
	return err
}
