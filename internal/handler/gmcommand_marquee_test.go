package handler

import (
	"testing"

	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestGMMarqueeBroadcastsJavaPacketAcrossMapsAndInstances(t *testing.T) {
	ws := world.NewState()
	for i := 1; i <= 3; i++ {
		sess := newHandlerTestSession(t, uint64(i))
		ws.AddPlayer(&world.PlayerInfo{
			CharID: int32(i), SessionID: sess.ID, Session: sess,
			MapID: int16(i), ShowID: int32(i * 100),
		})
	}
	// 非玩家連線的世界狀態不可造成全服廣播中斷。
	ws.AddPlayer(&world.PlayerInfo{CharID: 4, SessionID: 4})
	gm := ws.GetBySession(1)
	deps := &Deps{World: ws}
	if !dispatchGMAdmin(gm.Session, gm, "marquee", []string{"\\f>恭喜玩家", "\\f3小明\\f>完成挑戰！"}, deps) {
		t.Fatal("marquee 未由管理指令處理")
	}
	for i := 1; i <= 3; i++ {
		packets := drainHandlerTestPackets(ws.GetBySession(uint64(i)).Session)
		wantCount := 1
		if i == 1 {
			wantCount = 2 // 發送者也收到廣播，另收到完成回覆。
		}
		if len(packets) != wantCount {
			t.Fatalf("session %d packets=%d, want %d", i, len(packets), wantCount)
		}
		red := packet.NewReader(packets[0])
		if red.Opcode() != packet.S_OPCODE_REDMESSAGE || red.ReadH() != 3440 || red.ReadC() != 1 {
			t.Fatalf("session %d 的中央跑馬燈封包標頭不符 Java: %v", i, packets[0])
		}
		if text := red.ReadS(); text != "\\f>恭喜玩家 \\f3小明\\f>完成挑戰！" {
			t.Fatalf("session %d 的色碼或文字遭修改: %q", i, text)
		}
		if i == 1 {
			ack := packet.NewReader(packets[1])
			if ack.Opcode() != packet.S_OPCODE_MESSAGE || ack.ReadC() != 9 || ack.ReadS() != "跑馬燈廣播已發送：\\f>恭喜玩家 \\f3小明\\f>完成挑戰！" {
				t.Fatalf("發送者未收到完成回覆: %v", packets[1])
			}
		}
	}
}

func TestGMMarqueeEmptyMessageDoesNotBroadcast(t *testing.T) {
	ws := world.NewState()
	gmSess := newHandlerTestSession(t, 1)
	otherSess := newHandlerTestSession(t, 2)
	ws.AddPlayer(&world.PlayerInfo{CharID: 1, SessionID: 1, Session: gmSess})
	ws.AddPlayer(&world.PlayerInfo{CharID: 2, SessionID: 2, Session: otherSess})
	gmAdminMarquee(gmSess, []string{" ", "\t"}, &Deps{World: ws})
	if packets := drainHandlerTestPackets(otherSess); len(packets) != 0 {
		t.Fatalf("空跑馬燈不得發送其他玩家，got=%v", packets)
	}
	packets := drainHandlerTestPackets(gmSess)
	if len(packets) != 1 || packets[0][0] != packet.S_OPCODE_MESSAGE {
		t.Fatalf("空跑馬燈應僅回覆用法，got=%v", packets)
	}
}
