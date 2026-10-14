package system

import (
	"time"

	coresys "github.com/l1jgo/server/internal/core/system"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// NpcRespawnSystem processes NPC delete timers and respawn timers each tick.
// Flow: NPC dies → DeleteTimer counts down → send S_RemoveObject →
// RespawnTimer counts down → respawn at spawn point. Phase 2 (Update).
type NpcRespawnSystem struct {
	world *world.State
	maps  *data.MapDataTable
	deps  *handler.Deps
}

func NewNpcRespawnSystem(ws *world.State, maps *data.MapDataTable, deps *handler.Deps) *NpcRespawnSystem {
	return &NpcRespawnSystem{world: ws, maps: maps, deps: deps}
}

func (s *NpcRespawnSystem) Phase() coresys.Phase { return coresys.PhaseUpdate }

func (s *NpcRespawnSystem) Update(_ time.Duration) {
	for _, npc := range s.world.NpcList() {
		if !npc.Dead {
			continue
		}

		// Phase 1: Delete timer — wait for death animation to finish before removing
		if npc.DeleteTimer > 0 {
			npc.DeleteTimer--
			if npc.DeleteTimer <= 0 {
				// 屍體消失 — 從 AOI 網格移除 + 通知客戶端
				s.world.NpcCorpseCleanup(npc)
				nearby := s.world.GetNearbyPlayersInShow(npc.X, npc.Y, npc.MapID, 0, npc.ShowID)
				rmData := handler.BuildRemoveObject(npc.ID)
				handler.BroadcastToPlayers(nearby, rmData)
			}
			continue // 等刪除階段完成才開始重生計時
		}

		// Phase 2: Respawn timer
		if npc.RespawnTimer > 0 {
			npc.RespawnTimer--
			if npc.RespawnTimer <= 0 {
				s.respawnNpc(npc)
			}
		}
	}
}

func (s *NpcRespawnSystem) respawnNpc(npc *world.NpcInfo) {
	rule := NpcSpawnRuleFromNpc(npc)
	spawnX, spawnY, ok := FindNpcSpawnPoint(rule, s.world, s.maps, npc.ID, nil)
	if !ok {
		npc.RespawnTimer = 25
		return
	}

	npc.Dead = false
	npc.HP = npc.MaxHP
	npc.MP = npc.MaxMP
	npc.X = spawnX
	npc.Y = spawnY
	npc.MapID = npc.SpawnMapID
	npc.SpawnX = spawnX
	npc.SpawnY = spawnY
	npc.AggroTarget = 0
	npc.HateList = nil // 清空仇恨列表
	npc.CompanionHate = nil
	npc.AttackTimer = 0
	npc.MoveTimer = 0
	npc.StuckTicks = 0
	npc.Paralyzed = false
	npc.Sleeped = false
	npc.ActiveDebuffs = nil
	npc.PoisonDmgAmt = 0
	npc.PoisonDmgTimer = 0
	npc.PoisonAttackerSID = 0
	ApplyNpcInitialHideLikeJava(npc)
	removeShapeChangeFromNpc(npc)

	// 重置聊天計時器（重生後重新觸發出現聊天）
	StopNpcChat(npc)
	npc.ChatFirstAttack = false
	npc.ChatAppearStarted = false

	// Set tile as blocked (map passability for NPC pathfinding)
	if s.maps != nil {
		s.maps.SetImpassable(npc.MapID, npc.X, npc.Y, true)
	}

	// Re-add to NPC AOI grid + entity grid
	s.world.NpcRespawn(npc)

	// 通知附近玩家：顯示 NPC + 封鎖格子
	nearby := s.world.GetNearbyPlayersInShow(npc.X, npc.Y, npc.MapID, 0, npc.ShowID)
	for _, viewer := range nearby {
		sendNpcPack(viewer.Session, npc)
	}

	// 群體隊長重生：重新生成隊員（Java: L1MobGroupSpawn.doSpawn）
	if npc.MobGroupID > 0 && s.deps.MobGroups != nil && s.deps.Npcs != nil {
		group := s.deps.MobGroups.Get(npc.MobGroupID)
		if group != nil {
			s.respawnMobGroup(npc, group)
		}
	}
}

// respawnMobGroup 隊長重生時重新生成群體隊員。
func (s *NpcRespawnSystem) respawnMobGroup(leader *world.NpcInfo, group *data.MobGroup) {
	groupInfo := &world.MobGroupInfo{
		Leader:             leader,
		Members:            []*world.NpcInfo{leader},
		RemoveGroupOnDeath: group.RemoveGroupIfLeaderDie,
	}
	leader.GroupInfo = groupInfo

	for _, minion := range group.Minions {
		if minion.NpcID == 0 || minion.Count == 0 {
			continue
		}
		mTmpl := s.deps.Npcs.Get(minion.NpcID)
		if mTmpl == nil {
			continue
		}
		for j := 0; j < minion.Count; j++ {
			rule := NpcSpawnRule{
				MapID: leader.MapID,
				X:     leader.X,
				Y:     leader.Y,
				LocX1: leader.X - 2,
				LocY1: leader.Y - 2,
				LocX2: leader.X + 3,
				LocY2: leader.Y + 3,
			}
			mx, my, ok := FindNpcSpawnPoint(rule, s.world, s.maps, 0, nil)
			if !ok {
				continue
			}

			mob := s.createMinion(mTmpl, mx, my, leader)
			s.world.AddNpc(mob)
			if s.maps != nil {
				s.maps.SetImpassable(mob.MapID, mob.X, mob.Y, true)
			}
			groupInfo.Members = append(groupInfo.Members, mob)

			// 通知附近玩家
			nearby := s.world.GetNearbyPlayersInShow(mob.X, mob.Y, mob.MapID, 0, mob.ShowID)
			for _, viewer := range nearby {
				sendNpcPack(viewer.Session, mob)
			}
		}
	}
}

// createMinion 從模板建立隊員 NPC。
func (s *NpcRespawnSystem) createMinion(tmpl *data.NpcTemplate, x, y int32, leader *world.NpcInfo) *world.NpcInfo {
	atkSpeed := tmpl.AtkSpeed
	moveSpeed := tmpl.PassiveSpeed
	if s.deps.SprTable != nil {
		gfx := int(tmpl.GfxID)
		if tmpl.AtkSpeed != 0 {
			if v := s.deps.SprTable.GetAttackSpeed(gfx, data.ActAttack); v > 0 {
				atkSpeed = int16(v)
			}
		}
		if tmpl.PassiveSpeed != 0 {
			if v := s.deps.SprTable.GetMoveSpeed(gfx, data.ActWalk); v > 0 {
				moveSpeed = int16(v)
			}
		}
	}
	mob := &world.NpcInfo{
		ID:                world.NextNpcID(),
		NpcID:             tmpl.NpcID,
		Impl:              tmpl.Impl,
		GfxID:             tmpl.GfxID,
		Name:              tmpl.Name,
		NameID:            tmpl.NameID,
		Level:             tmpl.Level,
		X:                 x,
		Y:                 y,
		MapID:             leader.MapID,
		Heading:           leader.Heading,
		HP:                tmpl.HP,
		MaxHP:             tmpl.HP,
		MP:                tmpl.MP,
		MaxMP:             tmpl.MP,
		AC:                tmpl.AC,
		STR:               tmpl.STR,
		DEX:               tmpl.DEX,
		Intel:             tmpl.INT,
		Exp:               tmpl.Exp,
		Lawful:            tmpl.Lawful,
		Size:              tmpl.Size,
		MR:                tmpl.MR,
		Undead:            tmpl.Undead,
		UndeadType:        tmpl.UndeadType,
		TurnUndeadable:    tmpl.EffectiveTurnUndeadable(),
		TurnUndeadableSet: true,
		Hard:              tmpl.Hard,
		CantResurrect:     tmpl.CantResurrect,
		Agro:              tmpl.Agro,
		Family:            tmpl.Family,
		AgroFamily:        tmpl.AgroFamily,
		AtkDmg:            int32(tmpl.Level) + int32(tmpl.STR)/3,
		Ranged:            tmpl.Ranged,
		AtkSpeed:          atkSpeed,
		AtkMagicSpeed:     tmpl.AtkMagicSpeed,
		SubMagicSpeed:     tmpl.SubMagicSpeed,
		MoveSpeed:         moveSpeed,
		PoisonAtk:         tmpl.PoisonAtk,
		FireRes:           tmpl.FireRes,
		WaterRes:          tmpl.WaterRes,
		WindRes:           tmpl.WindRes,
		EarthRes:          tmpl.EarthRes,
		WeakAttr:          tmpl.WeakAttr,
		WeaponRequired:    tmpl.WeaponRequired,
		SpawnX:            leader.SpawnX,
		SpawnY:            leader.SpawnY,
		SpawnMapID:        leader.SpawnMapID,
		IsMinion:          true,
		ShowID:            leader.ShowID,
	}
	ApplyNpcMinionHideLikeJava(mob, leader)
	return mob
}
