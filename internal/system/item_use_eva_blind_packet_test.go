package system

import (
	"testing"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// Java Potion.useBlessOfEva → S_SkillIconBlessOfEva(objectId, time)（opcode 126）
func TestApplyEvaBreathSendsSkillIconBlessOfEva(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "eva",
	})
	sys := newPacketBoxDropItemUseSystem(t, ws)

	sys.applyEvaBreath(player.Session, player, 1800, 0)

	packets := drainSkillTestPackets(player.Session)
	want := handler.BuildSkillIconBlessOfEva(player.CharID, 1800)
	if !hasExactPacket(packets, want) {
		t.Fatalf("伊娃的祝福應送 S_SkillIconBlessOfEva(126)，packets=%v want=%v", packets, want)
	}
	for _, pkt := range packets {
		if len(pkt) >= 2 && pkt[0] == 250 && pkt[1] == 44 {
			t.Fatalf("不應再送 PacketBox 子碼 44：%v", pkt)
		}
	}
}

// Java Potion.useBlindPotion → S_CurseBlind(1)（opcode 47 + writeH）
func TestApplyBlindPotionSendsCurseBlind(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "blind",
	})
	sys := newPacketBoxDropItemUseSystem(t, ws)

	sys.applyBlindPotion(player.Session, player, 60)

	packets := drainSkillTestPackets(player.Session)
	want := handler.BuildCurseBlind(1)
	if !hasExactPacket(packets, want) {
		t.Fatalf("致盲藥水應送 S_CurseBlind(47)，packets=%v want=%v", packets, want)
	}
	for _, pkt := range packets {
		if len(pkt) >= 2 && pkt[0] == 250 && pkt[1] == 45 {
			t.Fatalf("不應再送 PacketBox 子碼 45：%v", pkt)
		}
	}
	if player.GetBuff(handler.SkillCurseBlind) == nil {
		t.Fatal("致盲 buff 未建立")
	}
}
