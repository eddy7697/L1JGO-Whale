package system

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// Java L1PcInstance.onAction 在安全區仍執行 L1Attack.action()。
// 這裡驗證客戶端收到的戰鬥動作，不能將封包測試等同於 BGM 聽測。
func TestPvPAttackNotifiesClientInEveryZoneLikeJava(t *testing.T) {
	for _, tc := range []struct {
		name string
		tile byte
	}{{"normal", 15}, {"combat", 47}, {"safety", 31}} {
		t.Run(tc.name, func(t *testing.T) {
			s, attacker := newClientAttackTestSystem(t)
			pvp := newPvPShowIDTestSystem(t, s.deps.World)
			t.Cleanup(pvp.deps.Scripting.Close)
			dir := t.TempDir()
			mapList := filepath.Join(dir, "maps.yaml")
			if err := os.WriteFile(mapList, []byte("maps:\n  - map_id: 900\n    start_x: 100\n    end_x: 102\n    start_y: 100\n    end_y: 100\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "900.txt"), []byte(fmt.Sprintf("%d,%d,%d\n", tc.tile, tc.tile, tc.tile)), 0o600); err != nil {
				t.Fatal(err)
			}
			maps, err := data.LoadMapData(mapList, dir)
			if err != nil || maps.Count() != 1 {
				t.Fatalf("load test zone: %v", err)
			}
			s.deps = pvp.deps
			s.deps.Combat = s
			s.deps.PvP = pvp
			s.deps.MapData = maps
			attacker.Level, attacker.Str, attacker.Dex, attacker.HitMod = 80, 35, 35, 100
			target := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{
				SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002,
				X: 102, Y: 100, MapID: attacker.MapID, ShowID: attacker.ShowID,
				HP: 100000, MaxHP: 100000,
			})
			sendClientAttackForTest(s, attacker, packet.C_OPCODE_ATTACK, target.CharID, target.X, target.Y)
			s.Update(0)
			for _, recipient := range []*world.PlayerInfo{attacker, target} {
				attacks := clientAttackPackets(drainSkillTestPackets(recipient.Session))
				if len(attacks) != 1 {
					t.Fatalf("每次揮刀雙方都須收到一次動作: recipient=%d packets=%x", recipient.CharID, attacks)
				}
				pkt := attacks[0]
				if len(pkt) != 24 || pkt[0] != 30 || pkt[1] != 1 || binary.LittleEndian.Uint32(pkt[2:6]) != uint32(attacker.CharID) || binary.LittleEndian.Uint32(pkt[6:10]) != uint32(target.CharID) || pkt[12] != 2 || binary.LittleEndian.Uint32(pkt[13:17]) != 0 || pkt[17] != 0 {
					t.Fatalf("動作不符 Eddy S_AttackPacket: %x", pkt)
				}
				if got := int32(binary.LittleEndian.Uint16(pkt[10:12])); got != target.MaxHP-target.HP {
					t.Fatalf("動作傷害=%d，實際失血=%d", got, target.MaxHP-target.HP)
				}
			}
			if tc.name == "safety" && (target.HP != target.MaxHP || attacker.PinkName) {
				t.Fatal("安全區保留動作，但不能造成傷害或粉紅名")
			}
		})
	}
}
