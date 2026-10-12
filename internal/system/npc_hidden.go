package system

import (
	"math/rand"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// ApplyNpcInitialHideLikeJava 對齊 yiwei L1MonsterInstance.initHide()。
func ApplyNpcInitialHideLikeJava(npc *world.NpcInfo) {
	applyNpcInitialHideRoll(npc, rand.Intn(3))
}

func applyNpcInitialHideRoll(npc *world.NpcInfo, roll int) {
	if npc == nil {
		return
	}
	clearNpcHidden(npc)
	switch npc.NpcID {
	case 45061, 45161, 45181, 45455:
		if roll == 0 {
			setNpcHidden(npc, world.NpcHiddenSink, 13)
		}
	case 45045, 45126, 45134, 45281:
		if roll == 0 {
			setNpcHidden(npc, world.NpcHiddenSink, 4)
		}
	case 45067, 45090, 45264, 45321, 45445, 45452:
		setNpcHidden(npc, world.NpcHiddenFly, 4)
	case 45681:
		setNpcHidden(npc, world.NpcHiddenFly, 11)
	case 46107, 46108:
		if roll == 0 {
			setNpcHidden(npc, world.NpcHiddenSink, 13)
		}
	case 46125, 46126, 46127, 46128:
		setNpcHidden(npc, world.NpcHiddenIce, 4)
	case 97259:
		setNpcHidden(npc, world.NpcHiddenSink, 28)
	}
}

func ApplyNpcMinionHideLikeJava(minion, leader *world.NpcInfo) {
	if minion == nil || leader == nil {
		return
	}
	switch leader.HiddenStatus {
	case world.NpcHiddenSink:
		switch minion.NpcID {
		case 45061, 45161, 45181, 45455:
			setNpcHidden(minion, world.NpcHiddenSink, 13)
		case 45045, 45126, 45134, 45281:
			setNpcHidden(minion, world.NpcHiddenSink, 4)
		case 46107, 46108:
			setNpcHidden(minion, world.NpcHiddenSink, 13)
		}
	case world.NpcHiddenFly:
		switch minion.NpcID {
		case 45067, 45090, 45264, 45321, 45445, 45452:
			setNpcHidden(minion, world.NpcHiddenFly, 4)
		case 45681:
			setNpcHidden(minion, world.NpcHiddenFly, 11)
		case 46125, 46126, 46127, 46128:
			setNpcHidden(minion, world.NpcHiddenIce, 4)
		}
	}
}

func TryNpcHideOnDamageLikeJava(npc *world.NpcInfo, ws *world.State) bool {
	return tryNpcHideOnDamageRoll(npc, ws, rand.Intn(10))
}

func tryNpcHideOnDamageRoll(npc *world.NpcInfo, ws *world.State, roll int) bool {
	if npc == nil || ws == nil || npc.Dead || npc.HP <= 0 || npc.HiddenStatus != world.NpcHiddenNone {
		return false
	}
	if npc.MaxHP <= 0 || npc.MaxHP/3 <= npc.HP || roll != 0 {
		return false
	}
	switch npc.NpcID {
	case 45061, 45161, 45181, 45455:
	default:
		return false
	}

	npc.AggroTarget = 0
	ClearHateList(npc)
	setNpcHidden(npc, world.NpcHiddenSink, 13)

	nearby := npcHiddenViewers(ws, npc)
	for _, viewer := range nearby {
		handler.SendActionGfx(viewer.Session, npc.ID, 11)
		handler.SendNpcPack(viewer.Session, npc)
	}
	return true
}

func setNpcHidden(npc *world.NpcInfo, hiddenStatus int, actionStatus byte) {
	if npc == nil {
		return
	}
	npc.HiddenStatus = hiddenStatus
	npc.HiddenActionStatus = actionStatus
}

func clearNpcHidden(npc *world.NpcInfo) {
	if npc == nil {
		return
	}
	npc.HiddenStatus = world.NpcHiddenNone
	npc.HiddenActionStatus = 0
}

func npcBlocksDirectPlayerAttackLikeJava(npc *world.NpcInfo) bool {
	if npc == nil {
		return false
	}
	return npc.HiddenStatus == world.NpcHiddenSink || npc.HiddenStatus == world.NpcHiddenFly
}

func npcRejectsDamageWhileHiddenLikeJava(npc *world.NpcInfo) bool {
	if npc == nil {
		return false
	}
	switch npc.HiddenStatus {
	case world.NpcHiddenSink, world.NpcHiddenFly, world.NpcHiddenShellman:
		return true
	default:
		return false
	}
}

func npcBlocksSkillTargetLikeJava(npc *world.NpcInfo, skillID int32) bool {
	if npc == nil {
		return false
	}
	switch npc.HiddenStatus {
	case world.NpcHiddenSink:
		return !skillCanRevealSinkHiddenLikeJava(skillID)
	case world.NpcHiddenFly:
		return true
	default:
		return false
	}
}

func skillCanRevealSinkHiddenLikeJava(skillID int32) bool {
	switch skillID {
	case 13, 72, 194, 213:
		return true
	default:
		return false
	}
}

func npcShouldAppearForPlayerLikeJava(npc *world.NpcInfo, p *world.PlayerInfo) bool {
	if npc == nil || p == nil || p.IsInvisible() || p.MapID != npc.MapID {
		return false
	}
	switch npc.HiddenStatus {
	case world.NpcHiddenSink:
		return npc.HP == npc.MaxHP && chebyshev32(p.X, p.Y, npc.X, npc.Y) <= 2
	case world.NpcHiddenIce:
		return npc.HP < npc.MaxHP
	case world.NpcHiddenKirtas:
		return npc.HP == npc.MaxHP || npc.KirtasBarrierTicks > 75
	default:
		return false
	}
}

func npcAppearOnGroundLikeJava(npc *world.NpcInfo, ws *world.State, p *world.PlayerInfo) bool {
	if npc == nil || ws == nil || p == nil {
		return false
	}
	action := byte(4)
	if npc.HiddenStatus == world.NpcHiddenKirtas {
		action = clearNpcKirtasBarrierLikeJava(npc)
	} else {
		clearNpcHidden(npc)
		if npc.GfxID == 1245 || npc.PolyOriginalGfxID == 1245 {
			action = 11
		}
	}
	if !p.IsGM() {
		npc.AggroTarget = p.SessionID
	}

	nearby := npcHiddenViewers(ws, npc)
	for _, viewer := range nearby {
		handler.SendActionGfx(viewer.Session, npc.ID, action)
		handler.SendNpcPack(viewer.Session, npc)
	}
	return true
}

func clearNpcKirtasBarrierLikeJava(npc *world.NpcInfo) byte {
	action := byte(4)
	if npc == nil {
		return action
	}
	if npc.HasDebuff(mobSkillKirtasBarrier1) && !npc.HasDebuff(mobSkillKirtasBarrier4) {
		npc.RemoveDebuff(mobSkillKirtasBarrier1)
		action = 21
	} else if npc.HasDebuff(mobSkillKirtasBarrier2) && !npc.HasDebuff(mobSkillKirtasBarrier4) {
		npc.RemoveDebuff(mobSkillKirtasBarrier2)
		action = 41
	} else if npc.HasDebuff(mobSkillKirtasBarrier3) {
		npc.RemoveDebuff(mobSkillKirtasBarrier3)
		npc.RemoveDebuff(78)
		action = 5
	} else if npc.HasDebuff(mobSkillKirtasBarrier4) {
		npc.RemoveDebuff(mobSkillKirtasBarrier4)
		npc.RemoveDebuff(mobSkillKirtasBarrier1)
		npc.RemoveDebuff(mobSkillKirtasBarrier2)
		action = 25
	}
	npc.KirtasBarrierTicks = 0
	clearNpcHidden(npc)
	return action
}

func npcHiddenViewers(ws *world.State, npc *world.NpcInfo) []*world.PlayerInfo {
	return ws.GetNearbyPlayersInShow(npc.X, npc.Y, npc.MapID, 0, npc.ShowID)
}
