package system

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
)

func elfAttributeFixture(t *testing.T) (*SkillSystem, *world.PlayerInfo, *world.NpcInfo) {
	t.Helper()
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001,
		Name: "elf", ClassType: 2, X: 100, Y: 100, MapID: 4,
	})
	npc := &world.NpcInfo{ID: 2001, NpcID: 70000, X: 101, Y: 100, MapID: 4}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	var err error
	s.deps.Skills, err = data.LoadSkillTable("../../data/yaml/skill_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s.deps.ElfAttributes = s
	return s, p, npc
}

func elfAction(s *SkillSystem, p *world.PlayerInfo, npc *world.NpcInfo, action string) {
	w := packet.NewWriterWithOpcode(packet.C_OPCODE_HACTION)
	w.WriteD(npc.ID)
	w.WriteS(action)
	handler.HandleNpcAction(p.Session, packet.NewReader(w.Bytes()), s.deps)
}

func TestElfAttributeSelectionThroughNpcAction(t *testing.T) {
	for _, tc := range []struct {
		action  string
		attr    int16
		message byte
	}{
		{"fire", 2, 1}, {"water", 4, 2}, {"air", 8, 3}, {"earth", 1, 4},
	} {
		t.Run(tc.action, func(t *testing.T) {
			s, p, npc := elfAttributeFixture(t)
			elfAction(s, p, npc, tc.action)
			if p.ElfAttr != tc.attr || !p.Dirty {
				t.Fatalf("selection: attr=%d dirty=%v", p.ElfAttr, p.Dirty)
			}
			packets := drainSkillTestPackets(p.Session)
			if len(packets) != 2 || !bytes.Equal(packets[0], []byte{250, 15, tc.message, 0, 0, 0, 0, 0}) || packets[1][0] != 39 {
				t.Fatalf("selection message/dialog=%v", packets)
			}
			p.Dirty = false
			elfAction(s, p, npc, "fire")
			if p.ElfAttr != tc.attr || p.Dirty || len(drainSkillTestPackets(p.Session)) != 0 {
				t.Fatal("must reset before selecting again")
			}
		})
	}
}

func TestElfAttributeResetRetainsNeutralSpellsAndRevertsProtection(t *testing.T) {
	s, p, npc := elfAttributeFixture(t)
	p.ElfAttr = 2
	p.FireRes = 7
	p.KnownSpells = []int32{4, 129, 133, 137, 138, 147, 148, 149, 150, 151, 152, 153, 154, 155, 160, 165, 171, 175, 176, 181}
	want := []int32{4, 129, 133, 137, 138, 147, 153, 154, 181}
	s.applyBuffEffect(p, &data.SkillInfo{SkillID: 147, BuffDuration: 64})
	if p.FireRes != 57 {
		t.Fatalf("protection setup: %d", p.FireRes)
	}
	drainSkillTestPackets(p.Session)
	elfAction(s, p, npc, "init")
	if p.ElfAttr != 0 || !slices.Equal(p.KnownSpells, want) || p.FireRes != 7 || p.HasBuff(147) || !p.Dirty {
		t.Fatalf("reset attr=%d spells=%v want=%v fire=%d buff=%v", p.ElfAttr, p.KnownSpells, want, p.FireRes, p.HasBuff(147))
	}
	var del []byte
	for _, pkt := range drainSkillTestPackets(p.Session) {
		if pkt[0] == 160 {
			del = pkt
		}
	}
	wantDel := make([]byte, 40) // protocol writer pads packets to eight-byte blocks
	wantDel[0], wantDel[1] = 160, 32
	wantDel[20], wantDel[21], wantDel[22], wantDel[23] = 248, 252, 252, 255
	if !bytes.Equal(del, wantDel) {
		t.Fatalf("S_DelSkill=%v", del)
	}
	elfAction(s, p, npc, "water")
	if p.ElfAttr != 4 || p.FireRes != 7 {
		t.Fatal("reset must allow another element without stat drift")
	}
}

func TestElfAttributeSaveFailurePreservesState(t *testing.T) {
	s, p, npc := elfAttributeFixture(t)
	pool, err := pgxpool.New(context.Background(), "postgres://unused@127.0.0.1:1/unused")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close() // deterministic repository failure without a database connection
	s.deps.CharRepo = persist.NewCharacterRepo(&persist.DB{Pool: pool})
	p.ElfAttr, p.FireRes = 2, 57
	p.KnownSpells = []int32{129, 148}
	p.AddBuff(&world.ActiveBuff{SkillID: 147, DeltaFireRes: 50, TicksLeft: 100})
	elfAction(s, p, npc, "init")
	if p.ElfAttr != 2 || p.FireRes != 57 || !p.HasBuff(147) || !slices.Equal(p.KnownSpells, []int32{129, 148}) || p.Dirty {
		t.Fatal("failed persistence must not reset in-memory element, skills, or resistance")
	}
	if len(drainSkillTestPackets(p.Session)) != 0 {
		t.Fatal("failed persistence acknowledged success")
	}
}

func TestElfAttributeRejectsInvalidInteractions(t *testing.T) {
	for _, scenario := range []string{"not elf", "dead", "other map", "other show", "too far", "reset neutral"} {
		t.Run(scenario, func(t *testing.T) {
			s, p, npc := elfAttributeFixture(t)
			action := "earth"
			switch scenario {
			case "not elf":
				p.ClassType = 3
			case "dead":
				p.Dead = true
			case "other map":
				npc.MapID = 5
			case "other show":
				npc.ShowID = 2
			case "too far":
				npc.X = 106
			case "reset neutral":
				action = "init"
			}
			elfAction(s, p, npc, action)
			if p.ElfAttr != 0 || p.Dirty || len(drainSkillTestPackets(p.Session)) != 0 {
				t.Fatal("invalid interaction changed player")
			}
		})
	}
}
