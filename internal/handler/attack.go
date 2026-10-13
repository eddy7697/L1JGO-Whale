package handler

import (
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// CalcHeading 計算兩點間的朝向（handler 內部及 system 使用）。
func CalcHeading(sx, sy, tx, ty int32) int16 {
	ddx := tx - sx
	ddy := ty - sy
	if ddx > 0 {
		ddx = 1
	} else if ddx < 0 {
		ddx = -1
	}
	if ddy > 0 {
		ddy = 1
	} else if ddy < 0 {
		ddy = -1
	}
	for i := int16(0); i < 8; i++ {
		if headingDX[i] == ddx && headingDY[i] == ddy {
			return i
		}
	}
	return 0
}

// FindArrow 在玩家背包中找到可用的箭矢（handler 內部及 system 使用）。
func FindArrow(player *world.PlayerInfo, deps *Deps) *world.InvItem {
	for _, item := range player.Inv.Items {
		info := deps.Items.Get(item.ItemID)
		if info != nil && info.ItemType == "arrow" && item.Count > 0 {
			return item
		}
	}
	return nil
}

// HandleAttack processes C_ATTACK (opcode 229).
// Thin handler: parse packet → queue to CombatSystem (Phase 2).
// Format: [D targetID][H x][H y]
func HandleAttack(sess *net.Session, r *packet.Reader, deps *Deps) {
	queueClientAttack(sess, r, deps, true)
}

func queueClientAttack(sess *net.Session, r *packet.Reader, deps *Deps, melee bool) {
	// 攻擊必須包含完整的 D/H/H；截短封包不能變成 target=0 的空揮。
	if r.Remaining() < 8 {
		return
	}
	player := deps.World.GetBySession(sess.ID)
	if player == nil || player.Dead || player.Paralyzed || player.Sleeped {
		return
	}
	// 負重過重無法攻擊（Java C_Attack：getWeight242() >= 197 → S_ServerMessage(110) 後 return）。
	if player.Inv != nil && player.Inv.Weight242(world.PlayerMaxWeight(player)) >= 197 {
		sendServerMessage(sess, 110)
		return
	}

	targetID := r.ReadD()
	x := int32(r.ReadH())
	y := int32(r.ReadH())

	if deps.Combat == nil {
		return
	}
	deps.Combat.QueueAttack(AttackRequest{
		AttackerSessionID: sess.ID,
		TargetID:          targetID,
		IsMelee:           melee,
		TargetX:           x,
		TargetY:           y,
		FromClient:        true,
	})
}

// HandleFarAttack processes C_FAR_ATTACK (opcode 123) — bow/ranged attacks.
// Thin handler: parse packet → queue to CombatSystem (Phase 2).
// Format: [D targetID][H x][H y]
func HandleFarAttack(sess *net.Session, r *packet.Reader, deps *Deps) {
	queueClientAttack(sess, r, deps, false)
}
