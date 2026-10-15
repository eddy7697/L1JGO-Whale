package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

type petTargetFixture struct {
	deps   *handler.Deps
	life   *PetSystem
	ai     *CompanionAISystem
	master *world.PlayerInfo
	victim *world.PlayerInfo
	pet    *world.PetInfo
	npc    *world.NpcInfo
	other  *world.PetInfo
	summon *world.SummonInfo
}

func newPetTargetFixture(t *testing.T) *petTargetFixture {
	t.Helper()
	ws := world.NewState()
	f := &petTargetFixture{deps: &handler.Deps{World: ws}}
	f.master = addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "owner",
		X: 100, Y: 100, MapID: 900, ShowID: 7, HP: 500, MaxHP: 500, Level: 50,
	})
	f.victim = addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "opponent",
		X: 101, Y: 100, MapID: 900, ShowID: 7, HP: 1000, MaxHP: 1000, Level: 50,
	})
	f.pet = &world.PetInfo{
		ID: 2001, OwnerCharID: f.master.CharID, X: 100, Y: 100, MapID: 900, ShowID: 7,
		HP: 500, MaxHP: 500, Level: 40, STR: 20, AtkDmg: 80, Ranged: 1, Status: world.PetStatusAggressive,
	}
	f.npc = &world.NpcInfo{ID: 3001, NpcID: 45001, Impl: "L1Monster", X: 101, Y: 100, MapID: 900, ShowID: 7, HP: 1000, MaxHP: 1000, Level: 1}
	f.other = &world.PetInfo{ID: 3002, OwnerCharID: f.victim.CharID, X: 101, Y: 100, MapID: 900, ShowID: 7, HP: 1000, MaxHP: 1000, Status: world.PetStatusRest}
	f.summon = &world.SummonInfo{ID: 3003, OwnerCharID: f.victim.CharID, X: 101, Y: 100, MapID: 900, ShowID: 7, HP: 1000, MaxHP: 1000}
	ws.AddPet(f.pet)
	ws.AddNpc(f.npc)
	ws.AddPet(f.other)
	ws.AddSummon(f.summon)
	f.life = NewPetSystem(f.deps)
	f.deps.PetLife = f.life
	f.ai = NewCompanionAISystem(ws, f.deps)
	return f
}

func (f *petTargetFixture) selectTarget(id int32) {
	w := packet.NewWriterWithOpcode(223)
	w.WriteD(f.pet.ID)
	w.WriteC(0)
	w.WriteD(id)
	handler.HandleSelectTarget(f.master.Session, packet.NewReader(w.RawBytes()), f.deps)
}

func TestPetSelectTargetReachesCombatForEveryTargetType(t *testing.T) {
	for _, mode := range []world.PetStatus{world.PetStatusAggressive, world.PetStatusAlert} {
		for _, kind := range []string{"monster", "player", "pet", "summon"} {
			t.Run(fmt.Sprintf("%d/%s", mode, kind), func(t *testing.T) {
				f := newPetTargetFixture(t)
				f.pet.Status = mode
				f.pet.HomeX, f.pet.HomeY = f.pet.X, f.pet.Y
				var id int32
				var hp *int32
				switch kind {
				case "monster":
					id, hp = f.npc.ID, &f.npc.HP
				case "player":
					id, hp = f.victim.CharID, &f.victim.HP
				case "pet":
					id, hp = f.other.ID, &f.other.HP
				case "summon":
					id, hp = f.summon.ID, &f.summon.HP
				}
				f.selectTarget(id)
				f.ai.tickPets()
				if *hp >= 1000 || f.pet.AttackTimer == 0 {
					t.Fatalf("selected %s was not attacked: hp=%d cooldown=%d", kind, *hp, f.pet.AttackTimer)
				}
				if kind != "monster" && f.npc.HP != 1000 {
					t.Fatal("autonomous scanning replaced the selected target")
				}
				if f.pet.AggroPetID != 0 {
					t.Fatal("normal selection must not use the match-only target slot")
				}
			})
		}
	}
}

func TestPetSelectTargetHonorsModesAndRejectsInvalidTargets(t *testing.T) {
	for _, mode := range []world.PetStatus{world.PetStatusDefensive, world.PetStatusRest, world.PetStatusExtend, world.PetStatusDismiss, world.PetStatusWhistle} {
		f := newPetTargetFixture(t)
		f.pet.Status = mode
		f.selectTarget(f.victim.CharID)
		if f.pet.AggroPlayerID != 0 || f.pet.Status != mode {
			t.Fatalf("manual targeting changed mode %d", mode)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*petTargetFixture) int32
	}{
		{"missing", func(f *petTargetFixture) int32 { return 999999 }},
		{"self", func(f *petTargetFixture) int32 { return f.pet.ID }},
		{"master", func(f *petTargetFixture) int32 { return f.master.CharID }},
		{"own pet", func(f *petTargetFixture) int32 { f.other.OwnerCharID = f.master.CharID; return f.other.ID }},
		{"own summon", func(f *petTargetFixture) int32 { f.summon.OwnerCharID = f.master.CharID; return f.summon.ID }},
		{"dead player", func(f *petTargetFixture) int32 { f.victim.Dead = true; return f.victim.CharID }},
		{"invisible", func(f *petTargetFixture) int32 { f.victim.Invisible = true; return f.victim.CharID }},
		{"gm invisible", func(f *petTargetFixture) int32 { f.victim.GMInvisible = true; return f.victim.CharID }},
		{"teleport", func(f *petTargetFixture) int32 { f.victim.HasTeleport = true; return f.victim.CharID }},
		{"player map", func(f *petTargetFixture) int32 { f.victim.MapID++; return f.victim.CharID }},
		{"player instance", func(f *petTargetFixture) int32 { f.victim.ShowID++; return f.victim.CharID }},
		{"monster instance", func(f *petTargetFixture) int32 { f.npc.ShowID++; return f.npc.ID }},
		{"pet map", func(f *petTargetFixture) int32 { f.other.MapID++; return f.other.ID }},
		{"summon instance", func(f *petTargetFixture) int32 { f.summon.ShowID++; return f.summon.ID }},
		{"dead pet", func(f *petTargetFixture) int32 { f.other.Dead = true; return f.other.ID }},
		{"dead summon", func(f *petTargetFixture) int32 { f.summon.HP = 0; return f.summon.ID }},
		{"far away", func(f *petTargetFixture) int32 { f.victim.X += 30; return f.victim.CharID }},
		{"merchant", func(f *petTargetFixture) int32 { f.npc.Impl = "L1Merchant"; return f.npc.ID }},
		{"hidden monster", func(f *petTargetFixture) int32 { f.npc.HiddenStatus = world.NpcHiddenSink; return f.npc.ID }},
		{"holy water", func(f *petTargetFixture) int32 { f.npc.NpcID = 45912; return f.npc.ID }},
		{"quest weapon", func(f *petTargetFixture) int32 { f.npc.WeaponRequired = 999; return f.npc.ID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPetTargetFixture(t)
			f.selectTarget(tc.mutate(f))
			if f.pet.AggroTarget != 0 || f.pet.AggroPlayerID != 0 {
				t.Fatal("invalid target was accepted")
			}
			packets := drainSkillTestPackets(f.master.Session)
			if len(packets) != 1 || packets[0][0] != packet.S_OPCODE_MESSAGE_CODE || packets[0][1] != 0x48 || packets[0][2] != 1 {
				t.Fatalf("expected invalid target message 328: %x", packets)
			}
		})
	}
}

func petTargetZoneMap(t *testing.T, attackerTile, victimTile byte) *data.MapDataTable {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "maps.yaml")
	if err := os.WriteFile(path, []byte("maps:\n  - map_id: 900\n    start_x: 100\n    end_x: 101\n    start_y: 100\n    end_y: 100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "900.txt"), []byte(fmt.Sprintf("%d,%d\n", attackerTile, victimTile)), 0o600); err != nil {
		t.Fatal(err)
	}
	maps, err := data.LoadMapData(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	return maps
}

func TestPetSelectedTargetRevalidatesSafetyAndInstanceAtAttackTime(t *testing.T) {
	for _, kind := range []string{"player", "pet", "summon"} {
		for _, attackerSafe := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/attackerSafe=%v", kind, attackerSafe), func(t *testing.T) {
				f := newPetTargetFixture(t)
				id := f.victim.CharID
				if kind == "pet" {
					id = f.other.ID
				}
				if kind == "summon" {
					id = f.summon.ID
				}
				f.selectTarget(id)
				if attackerSafe {
					f.deps.MapData = petTargetZoneMap(t, 31, 15)
				} else {
					f.deps.MapData = petTargetZoneMap(t, 15, 31)
				}
				f.ai.petAttackSelectedTarget(f.pet, id)
				if f.victim.HP != 1000 || f.other.HP != 1000 || f.summon.HP != 1000 || f.pet.AggroTarget != 0 || f.pet.AggroPlayerID != 0 {
					t.Fatal("safety zone transition must clear target without damage")
				}
				f.selectTarget(id)
				if f.pet.AggroTarget != 0 || f.pet.AggroPlayerID != 0 {
					t.Fatal("safety target re-selected")
				}
			})
		}
	}
	f := newPetTargetFixture(t)
	f.selectTarget(f.npc.ID)
	f.npc.ShowID++
	f.ai.petAttackTarget(f.pet)
	if f.npc.HP != 1000 || f.pet.AggroTarget != 0 {
		t.Fatal("NPC changed instance but remained attackable")
	}
	f.npc.ShowID--
	f.npc.Dead = true
	f.pet.AggroTarget = f.npc.ID
	f.ai.petAttackTarget(f.pet)
	if f.pet.AggroTarget != 0 {
		t.Fatal("dead NPC target was retained")
	}
}

func TestPetSelectedTargetChasesAndHonorsWalls(t *testing.T) {
	f := newPetTargetFixture(t)
	f.deps.World.UpdatePosition(f.victim.SessionID, 103, 100, 900, 0)
	f.deps.World.RemoveNpc(f.npc.ID)
	f.deps.World.RemovePet(f.other.ID)
	f.deps.World.RemoveSummon(f.summon.ID)
	f.deps.MapData = newSkillLOSTestMap(t)
	f.selectTarget(f.victim.CharID)
	f.ai.tickPets()
	if f.pet.X != 101 || f.victim.HP != 1000 {
		t.Fatalf("pet should move before attacking: x=%d hp=%d", f.pet.X, f.victim.HP)
	}
	f.pet.Ranged = 8
	f.pet.MoveTimer = 0
	// The map's wall at (102,100) prevents ranged damage.
	f.ai.tickPets()
	if f.victim.HP != 1000 {
		t.Fatal("pet attacked through wall")
	}
}

func TestPetSelectTargetPreservesMatchOpponent(t *testing.T) {
	f := newPetTargetFixture(t)
	f.pet.AggroPetID = f.other.ID
	f.selectTarget(f.victim.CharID)
	if f.pet.AggroPetID != f.other.ID || f.pet.AggroPlayerID != 0 {
		t.Fatal("manual target replaced match opponent")
	}
	f.ai.tickPets()
	if f.other.HP >= 1000 || f.victim.HP != 1000 {
		t.Fatal("match pet attack regressed")
	}
	f.pet.AggroPetID = 0
	f.master.MapID, f.pet.MapID, f.victim.MapID = 5125, 5125, 5125
	f.selectTarget(f.victim.CharID)
	if f.pet.AggroPlayerID != 0 {
		t.Fatal("manual targeting allowed inside match arena")
	}
}

func TestPetPlayerDamageHonorsInvulnerabilityAndReflection(t *testing.T) {
	for _, skill := range []int32{50, 78, 80, 157} {
		f := newPetTargetFixture(t)
		f.victim.AddBuff(&world.ActiveBuff{SkillID: skill, TicksLeft: 100})
		if skill == 78 {
			f.victim.AbsoluteBarrier = true
		}
		f.selectTarget(f.victim.CharID)
		f.ai.tickPets()
		if f.victim.HP != 1000 {
			t.Fatalf("pet bypassed invulnerability %d", skill)
		}
	}
	f := newPetTargetFixture(t)
	f.victim.Level = 200 // counter-barrier chance > 100%, deterministic
	f.victim.AddBuff(&world.ActiveBuff{SkillID: 91, TicksLeft: 100})
	f.selectTarget(f.victim.CharID)
	f.ai.tickPets()
	if f.victim.HP != 1000 || f.pet.HP >= 500 {
		t.Fatal("pet bypassed counter barrier")
	}
}

type petTargetDeath struct {
	handler.DeathManager
	calls int
}

func (d *petTargetDeath) KillPlayer(p *world.PlayerInfo) { d.calls++; p.Dead = true; p.FightId = 0 }

func TestPetTargetDeathsUseExistingLifecycleAndOwnerPKAttribution(t *testing.T) {
	for _, kind := range []string{"pet", "summon", "player", "duel"} {
		t.Run(kind, func(t *testing.T) {
			f := newPetTargetFixture(t)
			pvp := newPvPShowIDTestSystem(t, f.deps.World)
			t.Cleanup(pvp.deps.Scripting.Close)
			f.deps.Config, f.deps.Scripting, f.deps.Items, f.deps.Log = pvp.deps.Config, pvp.deps.Scripting, pvp.deps.Items, pvp.deps.Log
			pvp.deps = f.deps
			f.deps.PvP = pvp
			death := &petTargetDeath{}
			f.deps.Death = death
			id := f.victim.CharID
			switch kind {
			case "pet":
				id = f.other.ID
				f.other.HP, f.other.Exp = 1, 1000
			case "summon":
				id = f.summon.ID
				f.summon.HP = 1
			default:
				f.victim.HP = 1
			}
			if kind == "duel" {
				f.master.FightId, f.victim.FightId = f.victim.CharID, f.master.CharID
			}
			f.selectTarget(id)
			f.ai.tickPets()
			switch kind {
			case "pet":
				if !f.other.Dead || f.other.Exp != 950 {
					t.Fatal("pet death lifecycle not applied")
				}
			case "summon":
				if f.deps.World.GetSummon(id) != nil {
					t.Fatal("dead summon was not removed")
				}
			case "player":
				if death.calls != 1 || !f.victim.Dead || f.master.PKCount != 1 || f.master.Lawful >= 0 {
					t.Fatal("pet kill must charge its owner and kill victim once")
				}
			case "duel":
				if death.calls != 1 || f.master.PKCount != 0 {
					t.Fatal("duel kill must not become PK")
				}
			}
		})
	}
}

func TestPetTargetSpecialMonsterRequiresMasterStatus(t *testing.T) {
	f := newPetTargetFixture(t)
	f.npc.NpcID = 45912
	f.master.AddBuff(&world.ActiveBuff{SkillID: 1013, TicksLeft: 100})
	f.selectTarget(f.npc.ID)
	f.ai.tickPets()
	if f.npc.HP >= 1000 {
		t.Fatal("master holy water status did not permit target")
	}
	f.master.RemoveBuff(1013)
	f.pet.AttackTimer = 0
	hp := f.npc.HP
	f.ai.petAttackTarget(f.pet)
	if f.npc.HP != hp || f.pet.AggroTarget != 0 {
		t.Fatal("expired master status did not revoke target")
	}
}
