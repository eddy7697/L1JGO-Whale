package handler

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func newMovementProtocolPlayers(t *testing.T, language int) (*world.PlayerInfo, *world.PlayerInfo, *Deps) {
	t.Helper()
	ws := world.NewState()
	player := addMovementShowIDPlayer(ws, &world.PlayerInfo{
		CharID: 6001, X: 100, Y: 100, MapID: 4,
		SessionID: 1, Session: newMovementShowIDTestSession(t, 1),
	})
	viewer := addMovementShowIDPlayer(ws, &world.PlayerInfo{
		CharID: 6002, X: 110, Y: 100, MapID: 4,
		SessionID: 2, Session: newMovementShowIDTestSession(t, 2),
	})
	return player, viewer, &Deps{
		World: ws, Config: &config.Config{Server: config.ServerConfig{Language: language}},
	}
}

func TestMoveAcceptsPlainAndTaiwanHeadingsLikeJava(t *testing.T) {
	// 明文與 XOR 封包可由登入器決定；語系不能覆蓋 wire 值域判別。
	deltas := [8][2]int32{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}
	for _, language := range []int{3, 5} {
		for _, encoded := range []bool{false, true} {
			for heading, delta := range deltas {
				t.Run(fmt.Sprintf("language=%d/encoded=%t/heading=%d", language, encoded, heading), func(t *testing.T) {
					player, viewer, deps := newMovementProtocolPlayers(t, language)
					wireHeading := byte(heading)
					if encoded {
						wireHeading ^= 0x49
					}
					// 故意送遠方座標，確認只能依伺服器座標走一步。
					HandleMove(player.Session, moveReader(32000, 32000, wireHeading), deps)
					if player.X != 100+delta[0] || player.Y != 100+delta[1] || player.Heading != int16(heading) {
						t.Fatalf("heading=%d 解碼後位置錯誤：got=(%d,%d,h=%d)", wireHeading, player.X, player.Y, player.Heading)
					}
					packets := drainMovementShowIDPackets(viewer.Session)
					if len(packets) != 1 || len(packets[0]) < 10 || packets[0][0] != packet.S_OPCODE_MOVE_OBJECT {
						t.Fatalf("附近玩家應收到一步移動，packets=%v", packets)
					}
					pkt := packets[0]
					if int32(binary.LittleEndian.Uint32(pkt[1:5])) != player.CharID ||
						binary.LittleEndian.Uint16(pkt[5:7]) != 100 || binary.LittleEndian.Uint16(pkt[7:9]) != 100 || pkt[9] != byte(heading) {
						t.Fatalf("移動廣播應使用舊座標與已解碼朝向，packet=%v", pkt)
					}
					if packets := drainMovementShowIDPackets(player.Session); len(packets) != 0 {
						t.Fatalf("正常步伐不應回送自身移動或重建角色，packets=%v", packets)
					}
				})
			}
		}
	}
}

func TestMovePreservesConsecutiveStepsInOneInputBatch(t *testing.T) {
	for _, encoded := range []bool{false, true} {
		t.Run(fmt.Sprintf("encoded=%t", encoded), func(t *testing.T) {
			player, viewer, deps := newMovementProtocolPlayers(t, 3)
			// 重現同一個 InputSystem tick 內依序分派多包的情況。
			// 未修正前第二包起會因處理時間間隔過短而消失。
			player.LastMoveTime = time.Now().UnixNano()
			for step := 0; step < 6; step++ {
				heading := byte(2)
				if encoded {
					heading ^= 0x49
				}
				HandleMove(player.Session, moveReader(uint16(100+step), 100, heading), deps)
			}
			if player.X != 106 || player.Y != 100 {
				t.Fatalf("六個連續步伐必須全部執行，got=(%d,%d)", player.X, player.Y)
			}
			packets := drainMovementShowIDPackets(viewer.Session)
			if len(packets) != 6 {
				t.Fatalf("附近玩家應收到六個連續步伐，packets=%v", packets)
			}
			for step, pkt := range packets {
				if len(pkt) < 10 || pkt[0] != packet.S_OPCODE_MOVE_OBJECT || binary.LittleEndian.Uint16(pkt[5:7]) != uint16(100+step) {
					t.Fatalf("第 %d 步未依序廣播舊座標，packet=%v", step, pkt)
				}
			}
			if packets := drainMovementShowIDPackets(player.Session); len(packets) != 0 {
				t.Fatalf("連續移動不應回送 OwnCharPack 或自身移動，packets=%v", packets)
			}
		})
	}
}

func TestMoveRejectsMalformedWirePacketsWithoutSideEffects(t *testing.T) {
	full := []byte{packet.C_OPCODE_MOVE, 100, 0, 100, 0, 2}
	tests := map[string][]byte{}
	for length := 0; length < len(full); length++ {
		tests[fmt.Sprintf("truncated=%d", length)] = full[:length]
	}
	for _, heading := range []byte{8, 0x47, 0x50, 0xff} {
		pkt := append([]byte(nil), full...)
		pkt[5] = heading
		tests[fmt.Sprintf("invalid-heading=%d", heading)] = pkt
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			player, viewer, deps := newMovementProtocolPlayers(t, 3)
			HandleMove(player.Session, packet.NewReader(data), deps)
			if player.X != 100 || player.Y != 100 || player.Heading != 0 || player.LastMoveTime != 0 {
				t.Fatalf("無效封包不應改變角色狀態，got=(%d,%d,h=%d,time=%d)", player.X, player.Y, player.Heading, player.LastMoveTime)
			}
			if packets := drainMovementShowIDPackets(player.Session); len(packets) != 0 {
				t.Fatalf("無效封包不應回送資料，packets=%v", packets)
			}
			if packets := drainMovementShowIDPackets(viewer.Session); len(packets) != 0 {
				t.Fatalf("無效封包不應廣播資料，packets=%v", packets)
			}
		})
	}
}
