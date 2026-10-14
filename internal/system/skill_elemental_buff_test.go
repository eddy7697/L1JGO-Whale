package system

import (
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

func TestSkillElementalBuffElfElementalDefenseBuffsUseJavaValues(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "elf",
		X:         100,
		Y:         100,
		MapID:     4,
		ElfAttr:   2,
		MR:        5,
		Wis:       12,
		FireRes:   1,
		WaterRes:  2,
		WindRes:   3,
		EarthRes:  4,
	})
	s := newSkillTestSystem(t, ws)
	// 基礎 MR 依職業／精神／等級換算（Java: resetBaseMr）；淨化精神 WIS 12→15 會讓基礎 MR 再 +3。
	ResetBaseStats(player, s.deps)
	baseMR := player.MR

	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 129, BuffDuration: 960})
	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 137, BuffDuration: 960})
	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 138, BuffDuration: 960})
	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 147, BuffDuration: 64})

	if player.MR != baseMR+10+3 || player.Wis != 15 {
		t.Fatalf("魔法防禦/淨化精神應為 MR+10（另含精神換算 +3）、WIS+3，MR=%d(base %d) WIS=%d", player.MR, baseMR, player.Wis)
	}
	if player.FireRes != 61 || player.WaterRes != 12 || player.WindRes != 13 || player.EarthRes != 14 {
		t.Fatalf("屬性防禦應四屬 +10，單屬性防禦應依 ElfAttr 只加火抗 +50，四抗=%d/%d/%d/%d",
			player.FireRes, player.WaterRes, player.WindRes, player.EarthRes)
	}
}

func TestSkillElementalBuffElfWeaponAndBowBuffsUseJavaValues(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "elf",
		X:          100,
		Y:          100,
		MapID:      4,
		DmgMod:     1,
		HitMod:     2,
		BowHitMod:  3,
		BowDmgMod:  4,
		BraveSpeed: 0,
	})
	s := newSkillTestSystem(t, ws)

	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 148, BuffDuration: 960})
	if player.DmgMod != 5 || player.HitMod != 2 || player.BowHitMod != 3 || player.BowDmgMod != 4 {
		t.Fatalf("火焰武器應只給近戰傷害 +4，Dmg=%d Hit=%d BowHit=%d BowDmg=%d",
			player.DmgMod, player.HitMod, player.BowHitMod, player.BowDmgMod)
	}

	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 163, BuffDuration: 960})
	if player.HasBuff(148) || !player.HasBuff(163) || player.DmgMod != 7 || player.HitMod != 5 {
		t.Fatalf("烈炎武器應取代火焰武器並給近戰傷害 +6、命中 +3，buff148=%v buff163=%v Dmg=%d Hit=%d",
			player.GetBuff(148), player.GetBuff(163), player.DmgMod, player.HitMod)
	}

	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 149, BuffDuration: 960})
	if player.BowHitMod != 9 || player.BowDmgMod != 4 {
		t.Fatalf("風之神射應只給弓命中 +6，BowHit=%d BowDmg=%d", player.BowHitMod, player.BowDmgMod)
	}
	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 166, BuffDuration: 960})
	if player.HasBuff(149) || !player.HasBuff(166) || player.BowHitMod != 2 || player.BowDmgMod != 9 {
		t.Fatalf("暴風神射應取代風之神射並給弓傷害 +5、弓命中 -1，buff149=%v buff166=%v BowHit=%d BowDmg=%d",
			player.GetBuff(149), player.GetBuff(166), player.BowHitMod, player.BowDmgMod)
	}

	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 155, BuffDuration: 960})
	if player.BraveSpeed != 1 || player.BowHitMod != 2 || player.BowDmgMod != 9 {
		t.Fatalf("烈炎氣息應是勇敢速度 1，不應提供弓命中/傷害，Brave=%d BowHit=%d BowDmg=%d",
			player.BraveSpeed, player.BowHitMod, player.BowDmgMod)
	}
}

func TestSkillElementalBuffElfArmorAndWaterBuffsUseJavaValues(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "elf",
		X:         100,
		Y:         100,
		MapID:     4,
		AC:        10,
		WaterRes:  7,
	})
	s := newSkillTestSystem(t, ws)

	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 151, BuffDuration: 960})
	if player.AC != 4 {
		t.Fatalf("大地防護應依 Java 給 AC -6，AC=%d", player.AC)
	}
	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 159, BuffDuration: 960})
	if !player.HasBuff(151) || !player.HasBuff(159) || player.AC != 4 {
		t.Fatalf("大地的祝福在義維 Java 只送圖示不改 AC，且不在 REPEATEDSKILLS 任何群組不應移除大地防護，buff151=%v buff159=%v AC=%d",
			player.GetBuff(151), player.GetBuff(159), player.AC)
	}
	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 168, BuffDuration: 960})
	if player.HasBuff(151) || !player.HasBuff(159) || player.AC != 0 {
		t.Fatalf("鋼鐵防護依 Java REPEATEDSKILLS[1] 只與大地防護互斥，應移除 151 並保留 159，AC=10-10=0，buff151=%v buff159=%v AC=%d",
			player.GetBuff(151), player.GetBuff(159), player.AC)
	}

	s.applyBuffEffect(player, &data.SkillInfo{SkillID: 160, BuffDuration: 960})
	if player.AC != 0 || player.WaterRes != 7 || !player.HasBuff(160) {
		t.Fatalf("水之防護在義維 Java 只登錄狀態，不應改 AC/水抗，AC=%d WaterRes=%d buff160=%v",
			player.AC, player.WaterRes, player.GetBuff(160))
	}
}
