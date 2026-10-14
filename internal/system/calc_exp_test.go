package system

import (
	"testing"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

type stubPetLife struct {
	handler.PetLifecycleManager
	gained map[int32]int32
}

func (s *stubPetLife) AddPetExp(pet *world.PetInfo, expGain int32) {
	if s.gained == nil {
		s.gained = map[int32]int32{}
	}
	s.gained[pet.ID] += expGain
}

func newCalcExpTest(t *testing.T) (*handler.Deps, *stubPetLife) {
	t.Helper()
	ws := world.NewState()
	deps := newSkillTestSystem(t, ws).deps
	t.Cleanup(deps.Scripting.Close)
	deps.Config = &config.Config{Rates: config.RatesConfig{ExpRate: 1, LawfulRate: 1, PetExpRate: 1}, Gameplay: config.GameplayConfig{BaseAC: 10}}
	deps.PvP = NewPvPSystem(deps)
	pets := &stubPetLife{}
	deps.PetLife = pets
	return deps, pets
}

func addExpTestPlayer(t *testing.T, deps *handler.Deps, sid uint64, level int16) *world.PlayerInfo {
	t.Helper()
	return addSkillTestPlayer(deps.World, &world.PlayerInfo{
		SessionID: sid, Session: newSkillTestSession(t, sid), CharID: int32(1000 + sid),
		Level: level, Exp: int32(deps.Scripting.ExpForLevel(int(level))), X: 100, Y: 100, MapID: 4,
	})
}

func expGained(deps *handler.Deps, p *world.PlayerInfo) int32 {
	return p.Exp - int32(deps.Scripting.ExpForLevel(int(p.Level)))
}

// Java calcExp 未組隊：經驗、善惡依仇恨比例分給每位參與者，不只給 killer。
func TestNpcExpSplitsByHateAndLawfulLikeJava(t *testing.T) {
	deps, _ := newCalcExpTest(t)
	a := addExpTestPlayer(t, deps, 1, 10)
	b := addExpTestPlayer(t, deps, 2, 10)
	dead := addExpTestPlayer(t, deps, 3, 10)
	dead.Dead = true
	npc := &world.NpcInfo{ID: 2001, Exp: 1000, Lawful: 100, HateList: map[uint64]int32{1: 300, 2: 100, 3: 600}}
	distributeNpcExpLikeJava(a, npc, deps)
	if got := expGained(deps, a); got != 750 {
		t.Fatalf("killer 應得 1000*300/400=750, got %d", got)
	}
	if got := expGained(deps, b); got != 250 {
		t.Fatalf("協助者應得 250, got %d", got)
	}
	if expGained(deps, dead) != 0 {
		t.Fatal("死亡者不得分經驗")
	}
	// 善惡：NPC lawful 100 → 依仇恨反向分配 -75 / -25。
	if a.Lawful != -75 || b.Lawful != -25 {
		t.Fatalf("善惡分配錯誤: a=%d b=%d", a.Lawful, b.Lawful)
	}
}

// Java ExpTable.getPenaltyRate：70 級經驗除以 4，伺服器倍率仍套用。
func TestHighLevelExpPenaltyLikeJava(t *testing.T) {
	deps, _ := newCalcExpTest(t)
	deps.Config.Rates.ExpRate = 2
	p := addExpTestPlayer(t, deps, 1, 70)
	npc := &world.NpcInfo{ID: 2001, Exp: 1000, HateList: map[uint64]int32{1: 50}}
	distributeNpcExpLikeJava(p, npc, deps)
	if got := expGained(deps, p); got != 500 {
		t.Fatalf("70 級應得 1000*(1/4)*2=500, got %d", got)
	}
}

// Java calcExp 組隊：非隊員先依仇恨領走，隊伍份額乘隊員與王族加成後依等級平方分配。
func TestPartyExpDistributionLikeJava(t *testing.T) {
	deps, _ := newCalcExpTest(t)
	leader := addExpTestPlayer(t, deps, 1, 30) // 王族隊長
	leader.ClassType = 0
	member := addExpTestPlayer(t, deps, 2, 40)
	far := addExpTestPlayer(t, deps, 3, 20) // 隊員但不在視野
	far.X = 1000
	outsider := addExpTestPlayer(t, deps, 4, 10)
	party := deps.World.Parties.CreateParty(leader.CharID, member.CharID, world.PartyTypeNormal)
	deps.World.Parties.AddMember(party.LeaderID, far.CharID)

	npc := &world.NpcInfo{ID: 2001, Exp: 10000, Lawful: 1000, HateList: map[uint64]int32{1: 600, 2: 200, 4: 200}}
	distributeNpcExpLikeJava(leader, npc, deps)

	// 非隊員：10000*200/1000 = 2000。
	if got := expGained(deps, outsider); got != 2000 {
		t.Fatalf("非隊員應依仇恨領 2000, got %d", got)
	}
	// 隊伍份額 8000 × (1 + 0.04 一位可見隊員 + 0.059 王族隊長) = 8792；
	// 等級平方 900+1600=2500：隊長 8792*0.36=3165，隊員 8792*0.64=5626。
	if got := expGained(deps, leader); got != 3165 {
		t.Fatalf("隊長應得 3165, got %d", got)
	}
	if got := expGained(deps, member); got != 5626 {
		t.Fatalf("隊員應得 5626, got %d", got)
	}
	if expGained(deps, far) != 0 {
		t.Fatal("視野外隊員不得分經驗")
	}
	// 善惡：隊伍 800 依等級平方 → 隊長 -288、隊員 -512；非隊員 -200。
	if leader.Lawful != -288 || member.Lawful != -512 || outsider.Lawful != -200 {
		t.Fatalf("善惡分配錯誤: leader=%d member=%d outsider=%d", leader.Lawful, member.Lawful, outsider.Lawful)
	}
}

// Java calcExp：寵物依自身仇恨份額取得經驗（不套高等級懲罰），召喚獸份額不發放。
func TestPetAndSummonHateSharesLikeJava(t *testing.T) {
	deps, pets := newCalcExpTest(t)
	owner := addExpTestPlayer(t, deps, 1, 10)
	pet := &world.PetInfo{ID: 5001, OwnerCharID: owner.CharID, Level: 5}
	deps.World.AddPet(pet)
	sum := &world.SummonInfo{ID: 5002, OwnerCharID: owner.CharID}
	deps.World.AddSummon(sum)
	npc := &world.NpcInfo{ID: 2001, Exp: 1000, Lawful: 100}
	AddHate(npc, owner.SessionID, 500)
	AddCompanionHate(npc, pet.ID, owner.SessionID, 300)
	AddCompanionHate(npc, sum.ID, owner.SessionID, 200)
	if npc.HateList[owner.SessionID] != 1000 {
		t.Fatalf("寵物仇恨應併入主人: %d", npc.HateList[owner.SessionID])
	}
	distributeNpcExpLikeJava(owner, npc, deps)
	if got := expGained(deps, owner); got != 500 {
		t.Fatalf("主人應得本人仇恨份額 500, got %d", got)
	}
	if pets.gained[pet.ID] != 300 {
		t.Fatalf("寵物應得 300, got %d", pets.gained[pet.ID])
	}
	if owner.Lawful != -100 {
		t.Fatalf("善惡整份給主人: %d", owner.Lawful)
	}
	ClearHateList(npc)
	if npc.CompanionHate != nil {
		t.Fatal("清仇恨時應一併清除寵物仇恨")
	}
}

// 無仇恨列表（腳本直接擊殺）時全數給 killer；升級時重算基礎 AC/MR 並送出 S_SPMR。
func TestNpcDeathWithoutHateGivesKillerAndResetsBaseStats(t *testing.T) {
	deps, _ := newCalcExpTest(t)
	p := addExpTestPlayer(t, deps, 1, 49)
	p.ClassType, p.Dex, p.Wis = 1, 18, 12
	ResetBaseStats(p, deps)
	npc := &world.NpcInfo{ID: 2001, Exp: int32(deps.Scripting.ExpForLevel(50)) - p.Exp}
	awardGroupExpLikeJava(&expAcquisitor{player: p}, int64(npc.Exp), 0, npc, deps)
	if p.Level != 50 {
		t.Fatalf("應升到 50 級: %d", p.Level)
	}
	if p.BaseAC != CalcAcLikeJava(10, 50, 18) || p.AC != p.BaseAC || p.BaseMR != CalcBaseMrLikeJava(1, 50, 12) || p.MR != p.BaseMR {
		t.Fatalf("升級未重算基礎 AC/MR: AC=%d/%d MR=%d/%d", p.AC, p.BaseAC, p.MR, p.BaseMR)
	}
	if !hasServerPacketOpcode(drainSkillTestPackets(p.Session), 37) {
		t.Fatal("MR 改變時應送 S_SPMR (opcode 37)")
	}
}

func hasServerPacketOpcode(packets [][]byte, opcode byte) bool {
	for _, pkt := range packets {
		if len(pkt) > 0 && pkt[0] == opcode {
			return true
		}
	}
	return false
}
