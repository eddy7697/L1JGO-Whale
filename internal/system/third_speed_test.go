package system

import (
	"bytes"
	"testing"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

func thirdSpeedTestPlayers(t *testing.T) (*world.State, *world.PlayerInfo, *world.PlayerInfo, *world.PlayerInfo) {
	t.Helper()
	ws := world.NewState()
	players := make([]*world.PlayerInfo, 3)
	for i := range players {
		id := uint64(i + 1)
		players[i] = addSkillTestPlayer(ws, &world.PlayerInfo{
			SessionID: id, Session: newSkillTestSession(t, id), CharID: int32(1000 + id),
			X: 100, Y: 100, MapID: 4, ShowID: 10,
		})
	}
	players[2].ShowID = 20
	return ws, players[0], players[1], players[2]
}

func countThirdSpeedPackets(packets [][]byte, charID int32, liquorType byte) int {
	want := handler.BuildLiquor(charID, liquorType)
	count := 0
	for _, p := range packets {
		if bytes.Equal(p, want) {
			count++
		}
	}
	return count
}

func TestThirdSpeedPotionApplyUsesJavaPacketAndVisibleShow(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		name := "visible"
		if hidden {
			name = "GMInvisible"
		}
		t.Run(name, func(t *testing.T) {
			ws, player, sameShow, otherShow := thirdSpeedTestPlayers(t)
			player.GMInvisible = hidden
			deps := &handler.Deps{World: ws}
			deps.Skill = &SkillSystem{deps: deps}
			s := NewItemUseSystem(deps)
			s.applyThirdSpeed(player.Session, player, 600, 7976)
			if buff := player.ActiveBuffs[handler.SkillStatusThirdSpeed]; buff == nil || buff.TicksLeft != 3000 {
				t.Fatalf("third speed buff = %+v", buff)
			}
			if countThirdSpeedPackets(drainSkillTestPackets(player.Session), player.CharID, 8) != 1 {
				t.Fatal("self missing Java S_Liquor apply packet")
			}
			samePackets := drainSkillTestPackets(sameShow.Session)
			if hidden {
				if len(samePackets) != 0 {
					t.Fatalf("hidden GM leaked packets: %v", samePackets)
				}
			} else if countThirdSpeedPackets(samePackets, player.CharID, 8) != 1 {
				t.Fatal("same-show viewer missing third speed")
			}
			if packets := drainSkillTestPackets(otherShow.Session); len(packets) != 0 {
				t.Fatalf("third speed leaked across ShowID: %v", packets)
			}
		})
	}
}

func TestThirdSpeedRemovalClearsSelfAndVisibleShowOnce(t *testing.T) {
	cases := []struct {
		name   string
		remove func(*SkillSystem, *world.PlayerInfo)
	}{
		{"expiry", func(s *SkillSystem, p *world.PlayerInfo) { s.tickPlayerBuffs(p) }},
		{"explicit", func(s *SkillSystem, p *world.PlayerInfo) { s.removeBuffAndRevert(p, handler.SkillStatusThirdSpeed) }},
		{"cancellation", func(s *SkillSystem, p *world.PlayerInfo) { s.CancelAllBuffs(p) }},
		{"death", func(s *SkillSystem, p *world.PlayerInfo) { s.ClearAllBuffsOnDeath(p) }},
		{"gm-clear", func(s *SkillSystem, p *world.PlayerInfo) { s.GMClearAllStatuses(p) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws, player, sameShow, otherShow := thirdSpeedTestPlayers(t)
			s := newSkillTestSystem(t, ws)
			player.AddBuff(&world.ActiveBuff{SkillID: handler.SkillStatusThirdSpeed, TicksLeft: 1})
			tc.remove(s, player)
			if player.HasBuff(handler.SkillStatusThirdSpeed) {
				t.Fatal("third speed buff remains")
			}
			for _, receiver := range []*world.PlayerInfo{player, sameShow} {
				if countThirdSpeedPackets(drainSkillTestPackets(receiver.Session), player.CharID, 0) != 1 {
					t.Fatalf("player %d needs exactly one third-speed cancel", receiver.CharID)
				}
			}
			if packets := drainSkillTestPackets(otherShow.Session); len(packets) != 0 {
				t.Fatalf("third speed cancellation leaked across ShowID: %v", packets)
			}
			s.removeBuffAndRevert(player, handler.SkillStatusThirdSpeed)
			if packets := drainSkillTestPackets(player.Session); len(packets) != 0 {
				t.Fatalf("already removed speed sent duplicate cancellation: %v", packets)
			}
		})
	}
}

func TestThirdSpeedRemovalKeepsGMInvisiblePrivate(t *testing.T) {
	ws, player, sameShow, _ := thirdSpeedTestPlayers(t)
	player.GMInvisible = true
	player.AddBuff(&world.ActiveBuff{SkillID: handler.SkillStatusThirdSpeed, TicksLeft: 1})
	s := &SkillSystem{deps: &handler.Deps{World: ws}}
	s.tickPlayerBuffs(player)
	if countThirdSpeedPackets(drainSkillTestPackets(player.Session), player.CharID, 0) != 1 {
		t.Fatal("hidden GM missing self cancellation")
	}
	if packets := drainSkillTestPackets(sameShow.Session); len(packets) != 0 {
		t.Fatalf("hidden GM cancellation leaked: %v", packets)
	}
}
