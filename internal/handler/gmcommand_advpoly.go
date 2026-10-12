package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// AdvancedPolymorphStore 提供 GM 收錄操作；玩家卡冊與喚出介面另屬遊戲子系統。
type AdvancedPolymorphStore interface {
	Unlock(ctx context.Context, charID, polyID int32) (int32, error)
	Reset(ctx context.Context, charID int32) error
}

func gmAdminAdvPoly(sess *net.Session, cmd string, args []string, deps *Deps) {
	expected := 2
	if cmd == "advpolyreset" {
		expected = 1
	}
	if len(args) != expected {
		if cmd == "advpolyreset" {
			gmMsg(sess, "用法: .advpolyreset <線上角色>")
		} else if cmd == "advpolygive" {
			gmMsg(sess, "用法: .advpolygive <線上角色> <polyID|卡片ID>")
		} else {
			gmMsg(sess, "用法: .advpolyunlock <線上角色> <polyID>")
		}
		return
	}
	target := gmAdminTarget(sess, args[0], deps)
	if target == nil {
		return
	}
	if cmd == "advpolyreset" {
		if deps.AdvancedPoly == nil {
			gmMsg(sess, "進階變身收錄資料庫尚未初始化。")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := deps.AdvancedPoly.Reset(ctx, target.CharID); err != nil {
			gmAdminError(sess, deps, "進階變身收錄重置失敗", err)
			return
		}
		gmMsgf(sess, "已重置 %s 的進階變身收錄。", target.Name)
		return
	}
	id, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil || id <= 0 {
		gmMsg(sess, "polyID 或卡片 ID 必須是正整數。")
		return
	}
	if cmd == "advpolyunlock" {
		if _, ok := data.AdvancedPolymorphCard(int32(id)); !ok {
			gmMsg(sess, "polyID 不在進階變身清單中。")
			return
		}
		if deps.AdvancedPoly == nil {
			gmMsg(sess, "進階變身收錄資料庫尚未初始化。")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		count, err := deps.AdvancedPoly.Unlock(ctx, target.CharID, int32(id))
		if err != nil {
			gmAdminError(sess, deps, "進階變身解鎖失敗", err)
			return
		}
		gmMsgf(sess, "已解鎖 %s polyID=%d（累計 %d 次）。", target.Name, id, count)
		return
	}
	cardID := int32(id)
	if _, ok := data.AdvancedPolymorphID(cardID); !ok {
		var found bool
		cardID, found = data.AdvancedPolymorphCard(int32(id))
		if !found {
			gmMsg(sess, "無此進階變身 polyID 或卡片 ID。")
			return
		}
	}
	if deps.GMCmd == nil || deps.Items == nil || deps.Items.Get(cardID) == nil || target.Inv == nil {
		gmMsg(sess, "變身卡物品服務尚未初始化。")
		return
	}
	if target.Inv.IsFull() {
		gmMsg(sess, "發送失敗：目標玩家背包已滿。")
		return
	}
	before := gmAdvPolyCardCount(target, cardID)
	deps.GMCmd.GiveItem(target.Session, target, cardID, 1, 0)
	if gmAdvPolyCardCount(target, cardID) != before+1 {
		gmMsg(sess, "變身卡發送失敗。")
		return
	}
	gmMsgf(sess, "已發送變身卡 %d 給 %s。", cardID, target.Name)
	gmMsg(target.Session, "獲得進階變身卡（GM 發放）")
}

func gmAdvPolyCardCount(player *world.PlayerInfo, cardID int32) int64 {
	var count int64
	for _, item := range player.Inv.Items {
		if item != nil && item.ItemID == cardID {
			count += int64(item.Count)
		}
	}
	return count
}
