package system

import (
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

const (
	blankMagicScrollWizardClass int16  = 3
	msgMagicScrollCannotUse     uint16 = 281
)

func (s *ItemUseSystem) UseBlankMagicScroll(sess *net.Session, player *world.PlayerInfo, scroll *world.InvItem, selectedSkillIndex int) bool {
	if s == nil || s.deps == nil || s.deps.Items == nil || s.deps.Skills == nil || player == nil || player.Inv == nil || scroll == nil {
		return false
	}
	if player.ClassType != blankMagicScrollWizardClass {
		handler.SendServerMessage(sess, 264)
		return false
	}
	maxIndex, ok := blankMagicScrollMaxIndex(scroll.ItemID)
	if !ok || selectedSkillIndex < 0 || selectedSkillIndex > maxIndex {
		handler.SendServerMessage(sess, 591)
		return false
	}

	skillID := int32(selectedSkillIndex + 1)
	skill := s.deps.Skills.Get(skillID)
	outputItemID := int32(40859 + selectedSkillIndex)
	outputInfo := s.deps.Items.Get(outputItemID)
	if skill == nil || outputInfo == nil {
		handler.SendServerMessage(sess, 591)
		return false
	}

	if skill.HpConsume > 0 && player.HP <= int32(skill.HpConsume) {
		handler.SendServerMessage(sess, skillMsgNotEnoughHP)
		return false
	}
	if skill.MpConsume > 0 && player.MP < int32(skill.MpConsume) {
		handler.SendServerMessage(sess, skillMsgNotEnoughMP)
		return false
	}
	if skill.ItemConsumeID > 0 && skill.ItemConsumeCount > 0 {
		slot := player.Inv.FindByItemID(int32(skill.ItemConsumeID))
		if slot == nil || slot.Count < int32(skill.ItemConsumeCount) {
			handler.SendServerMessage(sess, 299)
			return false
		}
	}

	if skill.HpConsume > 0 {
		player.HP -= int32(skill.HpConsume)
		sendHpUpdate(sess, player)
	}
	if skill.MpConsume > 0 {
		player.SpendMP(int32(skill.MpConsume))
		sendMpUpdate(sess, player)
	}
	if skill.ItemConsumeID > 0 && skill.ItemConsumeCount > 0 {
		slot := player.Inv.FindByItemID(int32(skill.ItemConsumeID))
		if slot != nil {
			removed := player.Inv.RemoveItem(slot.ObjectID, int32(skill.ItemConsumeCount))
			if removed {
				handler.SendRemoveInventoryItem(sess, slot.ObjectID)
			} else {
				handler.SendItemCountUpdate(sess, slot)
			}
			handler.SendWeightUpdate(sess, player)
		}
	}

	s.consumeUsedItem(sess, player, scroll)
	s.addItemWithPacket(sess, player, outputInfo, 1)
	player.Dirty = true
	return true
}

func (s *ItemUseSystem) UseMagicScroll(sess *net.Session, player *world.PlayerInfo, scroll *world.InvItem, scrollInfo *data.ItemInfo, targetObjID int32, targetX, targetY int16) bool {
	if s == nil || s.deps == nil || s.deps.Items == nil || s.deps.Skills == nil || s.deps.Scripting == nil ||
		player == nil || player.Inv == nil || scroll == nil || scrollInfo == nil {
		return false
	}
	skillID, ok := magicScrollSkillID(scroll.ItemID)
	if !ok {
		return false
	}
	skill := s.deps.Skills.Get(skillID)
	if skill == nil {
		return false
	}

	now := time.Now()
	if now.Before(player.SkillDelayUntil) {
		return false
	}
	if !s.validateMagicScrollTarget(sess, player, scrollInfo, skill, targetObjID) {
		return false
	}

	s.consumeUsedItem(sess, player, scroll)
	skillSys := &SkillSystem{deps: s.deps}
	skillSys.castMagicScrollSkill(sess, player, skill, targetObjID, int32(targetX), int32(targetY), now)
	player.Dirty = true
	return true
}

func (s *ItemUseSystem) addItemWithPacket(sess *net.Session, player *world.PlayerInfo, itemInfo *data.ItemInfo, count int32) *world.InvItem {
	if s.deps.ItemCreate != nil {
		item, ok := s.deps.ItemCreate.GiveItem(sess, player, itemInfo.ItemID, count)
		if ok {
			return item
		}
		return nil
	}
	existing := player.Inv.FindByItemID(itemInfo.ItemID)
	added := player.Inv.AddItem(itemInfo.ItemID, count, itemInfo.Name, itemInfo.InvGfx, itemInfo.Weight, itemInfo.Stackable, byte(itemInfo.Bless))
	if existing != nil && added.ObjectID == existing.ObjectID {
		handler.SendItemCountUpdate(sess, added)
	} else {
		handler.SendAddItem(sess, added, itemInfo)
	}
	handler.SendWeightUpdate(sess, player)
	return added
}

func (s *ItemUseSystem) validateMagicScrollTarget(sess *net.Session, player *world.PlayerInfo, scrollInfo *data.ItemInfo, skill *data.SkillInfo, targetObjID int32) bool {
	switch scrollInfo.UseType {
	case "spell_buff":
		if targetObjID == 0 {
			handler.SendServerMessage(sess, msgMagicScrollCannotUse)
			return false
		}
	case "spell_long", "spell_short":
		if player.Invisible || targetObjID == 0 || targetObjID == player.CharID {
			handler.SendServerMessage(sess, msgMagicScrollCannotUse)
			return false
		}
		if !s.magicScrollTargetInRange(player, skill, targetObjID) {
			return false
		}
	case "dai":
		target := player.Inv.FindByObjectID(targetObjID)
		targetInfo := itemInfoForInvItem(s.deps.Items, target)
		if targetInfo == nil || targetInfo.Category != data.CategoryWeapon {
			handler.SendServerMessage(sess, msgMagicScrollCannotUse)
			return false
		}
	case "zel":
		target := player.Inv.FindByObjectID(targetObjID)
		targetInfo := itemInfoForInvItem(s.deps.Items, target)
		if targetInfo == nil || targetInfo.Category != data.CategoryArmor || targetInfo.Type != "armor" {
			handler.SendServerMessage(sess, msgMagicScrollCannotUse)
			return false
		}
	}
	return true
}

func (s *ItemUseSystem) magicScrollTargetInRange(player *world.PlayerInfo, skill *data.SkillInfo, targetObjID int32) bool {
	if s.deps.World == nil {
		return false
	}
	x, y, mapID, ok := s.magicScrollTargetPosition(targetObjID)
	if !ok || mapID != player.MapID {
		return false
	}
	dist := chebyshevDist(player.X, player.Y, x, y)
	if skill.Ranged > 0 {
		return dist <= int32(skill.Ranged)
	}
	return dist <= 20
}

func (s *ItemUseSystem) magicScrollTargetPosition(targetObjID int32) (int32, int32, int16, bool) {
	if target := s.deps.World.GetByCharID(targetObjID); target != nil && !target.Dead {
		return target.X, target.Y, target.MapID, true
	}
	if npc := s.deps.World.GetNpc(targetObjID); npc != nil && !npc.Dead {
		return npc.X, npc.Y, npc.MapID, true
	}
	if pet := s.deps.World.GetPet(targetObjID); pet != nil && !pet.Dead {
		return pet.X, pet.Y, pet.MapID, true
	}
	return 0, 0, 0, false
}

func itemInfoForInvItem(items *data.ItemTable, item *world.InvItem) *data.ItemInfo {
	if items == nil || item == nil {
		return nil
	}
	return items.Get(item.ItemID)
}

func blankMagicScrollMaxIndex(itemID int32) (int, bool) {
	switch itemID {
	case 40090:
		return 7, true
	case 40091:
		return 15, true
	case 40092:
		return 22, true
	case 40093:
		return 31, true
	case 40094:
		return 39, true
	default:
		return 0, false
	}
}

func magicScrollSkillID(itemID int32) (int32, bool) {
	switch itemID {
	case 49281:
		return 42, true
	case 49282:
		return 48, true
	case 49283:
		return 49, true
	case 49284:
		return 52, true
	case 49285:
		return 54, true
	case 49286:
		return 57, true
	case 40882:
		return 48, true
	}
	if itemID >= 40859 && itemID <= 40898 && itemID != 40863 {
		return itemID - 40858, true
	}
	return 0, false
}
