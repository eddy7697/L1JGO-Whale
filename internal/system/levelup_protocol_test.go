package system

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// Eddy L1PcInstance.levelUp 以 S_OwnCharStatus 通知新等級，前面沒有 S_Exp。
// 客戶端必須在完整狀態封包中仍能觀察到舊等級 -> 新等級的轉換。
func TestLevelUpFirstLevelNotificationIsFullStatusLikeJava(t *testing.T) {
	for _, levels := range [][2]int{{1, 2}, {1, 5}, {49, 50}} {
		t.Run(fmt.Sprintf("%d_to_%d", levels[0], levels[1]), func(t *testing.T) {
			ws := world.NewState()
			deps := newSkillTestSystem(t, ws).deps
			t.Cleanup(deps.Scripting.Close)
			p := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001,
				Level: int16(levels[0]), Exp: int32(deps.Scripting.ExpForLevel(levels[0])),
				ClassType: 1, Str: 16, Con: 16, Wis: 12, HP: 50, MaxHP: 100, MP: 5, MaxMP: 20,
			})
			exp := int32(deps.Scripting.ExpForLevel(levels[1]))
			gain := exp - p.Exp
			addExp(p, gain, deps)
			clientLevel := byte(levels[0])
			statusCount := 0
			for _, pkt := range drainSkillTestPackets(p.Session) {
				switch pkt[0] {
				case packet.S_OPCODE_EXP:
					t.Fatalf("升級不應先以 S_Exp 更新客戶端等級: %x", pkt)
				case packet.S_OPCODE_STATUS:
					statusCount++
					if clientLevel >= pkt[5] || pkt[5] != byte(levels[1]) {
						t.Fatalf("完整狀態未保留升級轉換: previous=%d packet=%x", clientLevel, pkt)
					}
					clientLevel = pkt[5]
					if len(pkt) != 56 || int32(binary.LittleEndian.Uint32(pkt[6:10])) != exp || int32(binary.LittleEndian.Uint32(pkt[20:24])) != p.MaxHP || int32(binary.LittleEndian.Uint32(pkt[28:32])) != p.MaxMP {
						t.Fatalf("升級狀態缺少最新 EXP/HP/MP: %x", pkt)
					}
				case packet.S_OPCODE_EFFECT:
					t.Fatal("Java 升級不額外發 S_SkillSound，不應猜測特效 ID")
				}
			}
			if statusCount != 1 || !p.Dirty || p.Level != int16(levels[1]) || p.HP != p.MaxHP || p.MP != p.MaxMP {
				t.Fatalf("升級未完整更新: status=%d level=%d dirty=%v HP=%d/%d MP=%d/%d", statusCount, p.Level, p.Dirty, p.HP, p.MaxHP, p.MP, p.MaxMP)
			}
		})
	}
}

func TestExpGainWithoutLevelUpDoesNotTriggerLevelTransition(t *testing.T) {
	ws := world.NewState()
	deps := newSkillTestSystem(t, ws).deps
	t.Cleanup(deps.Scripting.Close)
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001,
		Level: 10, Exp: int32(deps.Scripting.ExpForLevel(10)),
	})
	addExp(p, 1, deps)
	count := 0
	for _, pkt := range drainSkillTestPackets(p.Session) {
		if pkt[0] == packet.S_OPCODE_EXP {
			count++
			if pkt[1] != 10 || int32(binary.LittleEndian.Uint32(pkt[2:6])) != p.Exp {
				t.Fatalf("一般經驗更新錯誤: %x", pkt)
			}
		}
		if pkt[0] == packet.S_OPCODE_STATUS || pkt[0] == packet.S_OPCODE_EFFECT {
			t.Fatalf("未升級不應產生升級通知: %x", pkt)
		}
	}
	if count != 1 || p.Level != 10 {
		t.Fatalf("經驗更新次數=%d level=%d", count, p.Level)
	}
}
