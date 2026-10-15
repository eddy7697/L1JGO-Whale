package system

import (
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// SelectPetTarget mirrors C_SelectTarget / L1PetInstance.setMasterTarget.
// Pet food is not modelled yet; the existing pet hunger audit remains separate.
func (s *PetSystem) SelectPetTarget(sess *net.Session, player *world.PlayerInfo, pet *world.PetInfo, targetID int32) {
	if player == nil || pet == nil || pet.OwnerCharID != player.CharID ||
		(pet.Status != world.PetStatusAggressive && pet.Status != world.PetStatusAlert) {
		return
	}
	// Match opponents are assigned by PetMatchSystem, never by a client packet.
	if pet.AggroPetID != 0 || isPetMatchMap(pet.MapID) {
		return
	}
	target := resolvePetTarget(s.deps, s.deps.World, pet, targetID)
	if target == nil || chebyshev32(player.X, player.Y, target.x, target.y) > 18 {
		handler.SendServerMessage(sess, 328) // 請選擇正確的對象。
		return
	}
	pet.AggroTarget = 0
	pet.AggroPlayerID = 0
	if target.player != nil {
		pet.AggroPlayerID = targetID
	} else {
		pet.AggroTarget = targetID
	}
}

type petCombatTarget struct {
	x, y   int32
	npc    *world.NpcInfo
	player *world.PlayerInfo
	pet    *world.PetInfo
	summon *world.SummonInfo
}

func isPetMatchMap(mapID int16) bool {
	for _, id := range petMatchMapIDs {
		if id == mapID {
			return true
		}
	}
	return false
}

// Resolve again before every attack so teleport, death and safety-zone changes
// cannot leave a stale target attackable. PvP follows the current Go policy:
// safety zones block combat; the global Java ALT_NONPVP policy is not implemented.
func resolvePetTarget(deps *handler.Deps, ws *world.State, pet *world.PetInfo, id int32) *petCombatTarget {
	if pet == nil || pet.Dead || pet.HP <= 0 || id == 0 || id == pet.ID || isPetMatchMap(pet.MapID) {
		return nil
	}
	master := ws.GetByCharID(pet.OwnerCharID)
	if master == nil || master.Dead || master.HasTeleport || master.MapID != pet.MapID || master.ShowID != pet.ShowID {
		return nil
	}
	t := &petCombatTarget{}
	var mapID int16
	var showID int32
	var ownerID int32
	switch {
	case ws.GetByCharID(id) != nil:
		t.player = ws.GetByCharID(id)
		p := t.player
		if p.Dead || p.HP <= 0 || p.HasTeleport || p.IsInvisible() {
			return nil
		}
		t.x, t.y, mapID, showID, ownerID = p.X, p.Y, p.MapID, p.ShowID, p.CharID
	case ws.GetPet(id) != nil:
		t.pet = ws.GetPet(id)
		p := t.pet
		if p.Dead || p.HP <= 0 || p.AggroPetID != 0 {
			return nil
		}
		t.x, t.y, mapID, showID, ownerID = p.X, p.Y, p.MapID, p.ShowID, p.OwnerCharID
	case ws.GetSummon(id) != nil:
		t.summon = ws.GetSummon(id)
		p := t.summon
		if p.Dead || p.HP <= 0 {
			return nil
		}
		t.x, t.y, mapID, showID, ownerID = p.X, p.Y, p.MapID, p.ShowID, p.OwnerCharID
	case ws.GetNpc(id) != nil:
		t.npc = ws.GetNpc(id)
		n := t.npc
		if n.Dead || n.HP <= 0 || n.Impl != "L1Monster" || n.HiddenStatus != world.NpcHiddenNone || petMasterAttackMiss(master, n) {
			return nil
		}
		t.x, t.y, mapID, showID = n.X, n.Y, n.MapID, n.ShowID
	default:
		return nil
	}
	if mapID != pet.MapID || showID != pet.ShowID || ownerID == pet.OwnerCharID {
		return nil
	}
	if ownerID != 0 && deps.MapData != nil &&
		(deps.MapData.IsSafetyZone(pet.MapID, pet.X, pet.Y) || deps.MapData.IsSafetyZone(mapID, t.x, t.y)) {
		return nil
	}
	return t
}

// Java L1Character.isAttackMiss: special monsters require a master's status.
func petMasterAttackMiss(master *world.PlayerInfo, npc *world.NpcInfo) bool {
	var required int32
	switch npc.NpcID {
	case 45912, 45913, 45914, 45915:
		required = 1013 // STATUS_HOLY_WATER
	case 45916, 45675, 81082, 45625, 45674, 45685:
		required = 1014 // holy mithril powder / curse Yahee in this Java version
	case 45941, 45752, 45753:
		required = 1015 // Eva holy water / curse Barlog
	case 81341:
		required = 5008 // SECRET_MEDICINE_OF_DESTRUCTION
	}
	if required != 0 && !master.HasBuff(required) {
		return true
	}
	if (npc.NpcID >= 46068 && npc.NpcID <= 46091 && master.TempCharGfx == 6035) ||
		(npc.NpcID >= 46092 && npc.NpcID <= 46106 && master.TempCharGfx == 6034) {
		return true
	}
	if npc.WeaponRequired != 0 {
		if master.Equip.Weapon() == nil || !npc.CanReceiveDamageFrom(master.Equip.Weapon().ItemID) {
			return true
		}
	}
	return false
}

func (s *CompanionAISystem) petAttackSelectedTarget(pet *world.PetInfo, id int32) {
	target := resolvePetTarget(s.deps, s.world, pet, id)
	if target == nil {
		pet.AggroTarget, pet.AggroPlayerID = 0, 0
		return
	}
	if target.npc != nil {
		s.petAttackTarget(pet)
		return
	}
	if !s.petReachTarget(pet, target.x, target.y) {
		return
	}
	damage := petBaseDamage(pet)
	var victim *world.PlayerInfo
	switch {
	case target.player != nil:
		victim = target.player
		damage = petDamageAfterAC(damage, victim.AC)
		damage = applyImmuneToHarmDamage(victim, damage)
		damage = applyReductionArmorDamage(victim, damage, false)
		if playerDamageBlockedBySkm0LikeJava(victim) {
			damage = 0
		}
	case target.pet != nil:
		damage = petDamageAfterAC(damage, target.pet.AC)
		victim = s.world.GetByCharID(target.pet.OwnerCharID)
	case target.summon != nil:
		damage = petDamageAfterAC(damage, target.summon.AC)
		victim = s.world.GetByCharID(target.summon.OwnerCharID)
	}
	master := s.world.GetByCharID(pet.OwnerCharID)
	if damage > 0 && s.deps.PvP != nil && victim != nil {
		s.deps.PvP.TriggerPinkName(master, victim)
	}
	nearby := companionViewersAt(s.world, pet.X, pet.Y, pet.MapID, pet.ShowID)
	if target.player != nil && damage > 0 {
		damage = s.petPlayerReflectedDamage(pet, target.player, damage, nearby)
	}
	pet.Heading = calcNpcHeading(pet.X, pet.Y, target.x, target.y)
	handler.BroadcastToPlayers(nearby, buildNpcAttack(pet.ID, id, damage, pet.Heading))
	pet.AttackTimer = petAttackCooldown(pet)
	if damage <= 0 {
		return
	}
	switch {
	case target.player != nil:
		p := target.player
		if p.Sleeped {
			breakPlayerSleepBySkill(p)
		}
		p.ApplyDamage(damage)
		p.Dirty = true
		if p.HP < 0 {
			p.HP = 0
		}
		handler.SendHpUpdate(p.Session, p)
		if p.HP == 0 {
			isDuel := master.FightId == p.CharID && p.FightId == master.CharID
			if s.deps.Death != nil {
				s.deps.Death.KillPlayer(p)
			}
			if !isDuel {
				NewPvPSystem(s.deps).processPKKill(master, p)
			}
			pet.AggroPlayerID = 0
		}
	case target.pet != nil:
		p := target.pet
		p.HP -= damage
		p.Dirty = true
		if p.HP <= 0 {
			s.deps.PetLife.PetDie(p)
			pet.AggroTarget = 0
		} else if p.Status == world.PetStatusAggressive || p.Status == world.PetStatusDefensive || p.Status == world.PetStatusAlert {
			p.AggroTarget, p.AggroPlayerID = pet.ID, 0
		}
		if victim != nil {
			sendCompanionHpMeter(victim.Session, p.ID, p.HP, p.MaxHP)
		}
	case target.summon != nil:
		sum := target.summon
		sum.HP -= damage
		if sum.HP <= 0 {
			sum.HP, sum.Dead = 0, true
			NewSummonSystem(s.deps).killSummon(sum)
			pet.AggroTarget = 0
		} else if victim != nil {
			sendCompanionHpMeter(victim.Session, sum.ID, sum.HP, sum.MaxHP)
		}
	}
}

func (s *CompanionAISystem) petReachTarget(pet *world.PetInfo, x, y int32) bool {
	rangeTiles := int32(pet.Ranged)
	if rangeTiles < 1 {
		rangeTiles = 1
	}
	if chebyshev32(pet.X, pet.Y, x, y) > rangeTiles {
		if pet.MoveTimer <= 0 {
			s.companionMoveToward(pet.ID, pet.X, pet.Y, pet.MapID, pet.ShowID, x, y,
				func(id int32, newX, newY int32, h int16) { s.world.UpdatePetPosition(id, newX, newY, h) })
			pet.MoveTimer = 2
		}
		return false
	}
	return s.deps.MapData == nil || s.deps.MapData.HasLineOfSight(pet.MapID, pet.X, pet.Y, x, y)
}

func petBaseDamage(pet *world.PetInfo) int32 {
	damage := pet.AtkDmg + int32(pet.DamageByWeapon)
	if damage <= 0 {
		damage = int32(pet.Level)/2 + 1
	}
	damage += int32(world.RandInt(int(damage/4+1))) - damage/8
	if damage < 1 {
		damage = 1
	}
	return damage
}

func petDamageAfterAC(damage int32, ac int16) int32 {
	if ac < 0 {
		damage -= int32(-ac) / 3
	}
	if damage < 1 {
		damage = 1
	}
	return damage
}

func petAttackCooldown(pet *world.PetInfo) int {
	if pet.AtkSpeed <= 0 {
		return 10
	}
	if cooldown := int(pet.AtkSpeed) / 200; cooldown >= 3 {
		return cooldown
	}
	return 3
}

// Reuse the NPC-side reflection formula without creating a world NPC or giving
// the pet its master's weapon statistics or on-hit effects.
func (s *CompanionAISystem) petPlayerReflectedDamage(pet *world.PetInfo, target *world.PlayerInfo, damage int32, nearby []*world.PlayerInfo) int32 {
	reflected := int32(0)
	if pet.Ranged <= 1 && target.HasBuff(91) && world.RandInt(100)+1 <= 58+int(target.Level)-int(pet.Level) {
		reflected = int32((int(pet.STR)+int(pet.Level))<<1) * 3 / 2
		if reflected > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(target.CharID, 10710))
		}
	} else if pet.Ranged <= 1 && target.HasBuff(191) && world.RandInt(100) < 23 {
		reflected = 40
	}
	if reflected <= 0 {
		return damage
	}
	pet.HP -= reflected
	pet.Dirty = true
	if pet.HP <= 0 {
		s.deps.PetLife.PetDie(pet)
	}
	if master := s.world.GetByCharID(pet.OwnerCharID); master != nil {
		sendCompanionHpMeter(master.Session, pet.ID, pet.HP, pet.MaxHP)
	}
	return 0
}
