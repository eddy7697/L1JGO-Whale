package handler

import (
	"bytes"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net/packet"
)

// padTo8 比照 Java ServerBasePacket.getBytes 補滿 8 位元組。
func padTo8(b []byte) []byte {
	out := append([]byte{}, b...)
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	return out
}

// Java S_PacketBox(MSG_LEVEL_OVER, level)：[C 250][C 12][C 0][C level]
func TestBuildPacketBoxLevelOverMatchesJava(t *testing.T) {
	got := BuildPacketBoxLevelOver(45)
	want := padTo8([]byte{packet.S_OPCODE_EVENT, 12, 0, 45})
	if !bytes.Equal(got, want) {
		t.Fatalf("MSG_LEVEL_OVER 封包格式錯誤：got=%v want=%v", got, want)
	}
}

// Java S_CurseBlind(type)：[C 47][H type]
func TestBuildCurseBlindMatchesJava(t *testing.T) {
	got := BuildCurseBlind(1)
	want := padTo8([]byte{packet.S_OPCODE_CURSEBLIND, 1, 0})
	if !bytes.Equal(got, want) {
		t.Fatalf("S_CurseBlind 封包格式錯誤：got=%v want=%v", got, want)
	}
}

// Java S_SkillIconBlessOfEva(objectId, time)：[C 126][D objectId][H time]
func TestBuildSkillIconBlessOfEvaMatchesJava(t *testing.T) {
	got := BuildSkillIconBlessOfEva(0x01020304, 1800)
	want := padTo8([]byte{126, 0x04, 0x03, 0x02, 0x01, 0x08, 0x07})
	if !bytes.Equal(got, want) {
		t.Fatalf("S_SkillIconBlessOfEva 封包格式錯誤：got=%v want=%v", got, want)
	}
	if packet.S_OPCODE_BLESSOFEVA != 126 {
		t.Fatalf("S_OPCODE_BLESSOFEVA 應為 126，got=%d", packet.S_OPCODE_BLESSOFEVA)
	}
}

func TestCheckLevelRestrictionSendsLevelOverPacketBox(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	info := &data.ItemInfo{MinLevel: 10, MaxLevel: 49}

	if checkLevelRestriction(sess, 52, info) {
		t.Fatal("超過等級上限應被拒絕")
	}
	packets := drainHandlerTestPackets(sess)
	if len(packets) != 1 {
		t.Fatalf("應只送一個封包，got=%d", len(packets))
	}
	if want := BuildPacketBoxLevelOver(49); !bytes.Equal(packets[0], want) {
		t.Fatalf("等級上限應送 MSG_LEVEL_OVER：got=%v want=%v", packets[0], want)
	}

	// 等級不足仍走 318 「等級 %0 以上才可使用」
	if checkLevelRestriction(sess, 5, info) {
		t.Fatal("低於等級下限應被拒絕")
	}
	packets = drainHandlerTestPackets(sess)
	if len(packets) != 1 || packets[0][0] != packet.S_OPCODE_MESSAGE_CODE {
		t.Fatalf("等級不足應送 S_ServerMessage：%v", packets)
	}

	if !checkLevelRestriction(sess, 30, info) {
		t.Fatal("等級範圍內應允許")
	}
	if packets = drainHandlerTestPackets(sess); len(packets) != 0 {
		t.Fatalf("允許時不應送封包：%v", packets)
	}
}
