package handler

import (
	"testing"

	"github.com/l1jgo/server/internal/data"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func TestHandleUseItemBlockedByShockStunBeforeEquipmentDispatchLikeJava(t *testing.T) {
	ws := world.NewState()
	sess := newHandlerTestSession(t, 1)
	player := &world.PlayerInfo{
		SessionID: sess.ID,
		Session:   sess,
		CharID:    1001,
		Name:      "stunned",
		HP:        100,
		Level:     50,
		Paralyzed: true,
		Inv:       world.NewInventory(),
	}
	player.AddBuff(&world.ActiveBuff{SkillID: 87, TicksLeft: 25, SetParalyzed: true})
	weapon := player.Inv.AddItemWithID(7001, 1, 1, "sword", 0, 1000, true, 1)
	ws.AddPlayer(player)

	equip := &captureEquipManager{}
	deps := &Deps{
		World: ws,
		Items: loadRepairTestItems(t),
		Equip: equip,
		Log:   zap.NewNop(),
	}

	HandleUseItem(sess, useItemReader(weapon.ObjectID), deps)

	if equip.weaponCalls != 0 {
		t.Fatalf("Java C_ItemUSe 會以 isParalyzedX 在分派前擋下道具使用，不應委派 EquipWeapon，calls=%d", equip.weaponCalls)
	}
	if weapon.Equipped {
		t.Fatal("SHOCK_STUN 中使用武器不應改變裝備狀態")
	}
	if !hasHandlerTeleportUnlockPacket(drainHandlerTestPackets(sess)) {
		t.Fatal("Java C_ItemUSe 在 isParalyzedX 擋下道具使用時會送 TeleportUnlock")
	}
}

func TestHandleUseItemShockStunCancelsAbsoluteBarrierBeforeBlockLikeJava(t *testing.T) {
	ws := world.NewState()
	sess := newHandlerTestSession(t, 1)
	player := &world.PlayerInfo{
		SessionID:       sess.ID,
		Session:         sess,
		CharID:          1001,
		Name:            "stunned",
		HP:              100,
		Level:           50,
		Paralyzed:       true,
		AbsoluteBarrier: true,
		Inv:             world.NewInventory(),
	}
	player.AddBuff(&world.ActiveBuff{SkillID: 87, TicksLeft: 25, SetParalyzed: true})
	player.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 25, SetAbsoluteBarrier: true})
	weapon := player.Inv.AddItemWithID(7001, 1, 1, "sword", 0, 1000, true, 1)
	ws.AddPlayer(player)

	equip := &captureEquipManager{}
	skill := &captureSkillManager{}
	deps := &Deps{
		World: ws,
		Items: loadRepairTestItems(t),
		Equip: equip,
		Skill: skill,
		Log:   zap.NewNop(),
	}

	HandleUseItem(sess, useItemReader(weapon.ObjectID), deps)

	if skill.cancelAbsoluteBarrierCalls != 1 {
		t.Fatalf("Java C_ItemUSe 會先解除 ABSOLUTE_BARRIER 再以 isParalyzedX 擋下道具使用，CancelAbsoluteBarrier calls=%d", skill.cancelAbsoluteBarrierCalls)
	}
	if equip.weaponCalls != 0 {
		t.Fatalf("解除 ABSOLUTE_BARRIER 後仍應因 SHOCK_STUN 阻擋裝備分派，calls=%d", equip.weaponCalls)
	}
	if weapon.Equipped {
		t.Fatal("SHOCK_STUN 中使用武器不應改變裝備狀態")
	}
	if !hasHandlerTeleportUnlockPacket(drainHandlerTestPackets(sess)) {
		t.Fatal("Java C_ItemUSe 在 isParalyzedX 擋下道具使用時會送 TeleportUnlock")
	}
}

type captureEquipManager struct {
	weaponCalls int
	armorCalls  int
}

func (m *captureEquipManager) EquipWeapon(_ *l1net.Session, _ *world.PlayerInfo, _ *world.InvItem, _ *data.ItemInfo) {
	m.weaponCalls++
}

func (m *captureEquipManager) EquipArmor(_ *l1net.Session, _ *world.PlayerInfo, _ *world.InvItem, _ *data.ItemInfo) {
	m.armorCalls++
}

func (m *captureEquipManager) UnequipSlot(_ *l1net.Session, _ *world.PlayerInfo, _ world.EquipSlot) {
}

func (m *captureEquipManager) FindEquippedSlot(_ *world.PlayerInfo, _ *world.InvItem) world.EquipSlot {
	return world.SlotNone
}

func (m *captureEquipManager) RecalcEquipStats(_ *l1net.Session, _ *world.PlayerInfo) {
}

func (m *captureEquipManager) InitEquipStats(_ *world.PlayerInfo) {
}

func (m *captureEquipManager) SendEquipList(_ *l1net.Session, _ *world.PlayerInfo) {
}

func hasHandlerTeleportUnlockPacket(packets [][]byte) bool {
	for _, pkt := range packets {
		if len(pkt) >= 2 && pkt[0] == packet.S_OPCODE_PARALYSIS && pkt[1] == TeleportUnlock {
			return true
		}
	}
	return false
}
