package system

import (
	"math/rand"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

func TestSkillNpcDebuffRejectsDifferentShowNpcTargetLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    7501,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     900,
		ShowID:    70,
	})
	npc := &world.NpcInfo{
		ID:       7601,
		NpcID:    45000,
		Impl:     "L1Monster",
		Name:     "other-show-npc",
		X:        101,
		Y:        100,
		MapID:    900,
		ShowID:   99,
		WeakAttr: weakElementalFire,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.executeNpcDebuffSkill(caster.Session, caster, &data.SkillInfo{
		SkillID:  23,
		ActionID: 19,
		Ranged:   10,
	}, npc)

	if npc.HateList != nil && npc.HateList[caster.SessionID] != 0 {
		t.Fatalf("yiwei isTarget 會以 showId 不同擋下 NPC debuff，不應加仇恨，hate=%v", npc.HateList)
	}
	if hasActionGfxPacket(drainSkillTestPackets(caster.Session), caster.CharID, 19) {
		t.Fatal("不同 ShowID NPC 目標應在施法動畫前被拒絕")
	}
}

func TestSkillNpcDebuffBroadcastsOnlySameShowLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    7511,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     900,
		ShowID:    70,
	})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    7512,
		Name:      "same",
		X:         102,
		Y:         100,
		MapID:     900,
		ShowID:    70,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    7513,
		Name:      "other",
		X:         102,
		Y:         100,
		MapID:     900,
		ShowID:    99,
	})
	npc := &world.NpcInfo{
		ID:       7611,
		NpcID:    45000,
		Impl:     "L1Monster",
		Name:     "same-show-npc",
		X:        101,
		Y:        100,
		MapID:    900,
		ShowID:   70,
		WeakAttr: weakElementalFire,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.executeNpcDebuffSkill(caster.Session, caster, &data.SkillInfo{
		SkillID:  23,
		ActionID: 19,
		Ranged:   10,
	}, npc)

	samePackets := drainSkillTestPackets(sameShow.Session)
	if !hasActionGfxPacket(samePackets, caster.CharID, 19) {
		t.Fatal("同 ShowID 觀察者應收到 NPC debuff 施法動作")
	}
	if !hasSkillEffectPacket(samePackets, npc.ID, 2167) {
		t.Fatal("同 ShowID 觀察者應收到 NPC 弱火特效")
	}

	otherPackets := drainSkillTestPackets(otherShow.Session)
	if hasActionGfxPacket(otherPackets, caster.CharID, 19) {
		t.Fatal("不同 ShowID 觀察者不應收到 NPC debuff 施法動作")
	}
	if hasSkillEffectPacket(otherPackets, npc.ID, 2167) {
		t.Fatal("不同 ShowID 觀察者不應收到 NPC 弱點特效")
	}
}

func TestSkillNpcWindShackleExistingDebuffSendsYiweiEffectWithoutRefresh(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    7521,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     900,
		ShowID:    70,
	})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    7522,
		Name:      "same",
		X:         102,
		Y:         100,
		MapID:     900,
		ShowID:    70,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    7523,
		Name:      "other",
		X:         102,
		Y:         100,
		MapID:     900,
		ShowID:    99,
	})
	npc := &world.NpcInfo{
		ID:     7621,
		NpcID:  45000,
		Impl:   "L1Monster",
		Name:   "wind-shackle-npc",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 70,
	}
	npc.AddDebuff(167, 25)
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.executeNpcDebuffSkill(caster.Session, caster, &data.SkillInfo{
		SkillID:      167,
		Target:       "buff",
		BuffDuration: 16,
		ActionID:     19,
		CastGfx:      1799,
		Ranged:       8,
	}, npc)

	if got := npc.ActiveDebuffs[167]; got != 25 {
		t.Fatalf("Java WIND_SHACKLE 對 NPC 已有 167 時不刷新時間，got=%d want=25", got)
	}

	samePackets := drainSkillTestPackets(sameShow.Session)
	if !hasActionGfxPacket(samePackets, caster.CharID, 19) {
		t.Fatal("同 ShowID 觀察者應收到 NPC debuff 施法動作")
	}
	if !hasSkillEffectPacket(samePackets, npc.ID, 1799) {
		t.Fatalf("yiwei sendGrfx 對 NPC 已有 167 仍會送目標 S_SkillSound 1799，packets=%v", samePackets)
	}

	otherPackets := drainSkillTestPackets(otherShow.Session)
	if hasActionGfxPacket(otherPackets, caster.CharID, 19) {
		t.Fatal("不同 ShowID 觀察者不應收到 NPC debuff 施法動作")
	}
	if hasSkillEffectPacket(otherPackets, npc.ID, 1799) {
		t.Fatal("不同 ShowID 觀察者不應收到 NPC 風之枷鎖特效")
	}
}

func TestSkillNpcElementalDebuffsApplyToNpcLikeJava(t *testing.T) {
	tests := []struct {
		name    string
		skillID int32
		gfxID   int32
	}{
		{name: "pollute-water", skillID: 173, gfxID: 5830},
		{name: "striker-gale", skillID: 174, gfxID: 5826},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rand.Seed(1)
			ws := world.NewState()
			caster := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID:   1,
				Session:     newSkillTestSession(t, 1),
				CharID:      7531,
				AccessLevel: 200, // 此測試驗證效果與視野；成功率另由 Java 機率測試驗證。
				Name:        "caster",
				X:           100,
				Y:           100,
				MapID:       900,
				ShowID:      70,
				Level:       80,
				Intel:       30,
			})
			sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID: 2,
				Session:   newSkillTestSession(t, 2),
				CharID:    7532,
				Name:      "same",
				X:         102,
				Y:         100,
				MapID:     900,
				ShowID:    70,
			})
			otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID: 3,
				Session:   newSkillTestSession(t, 3),
				CharID:    7533,
				Name:      "other",
				X:         102,
				Y:         100,
				MapID:     900,
				ShowID:    99,
			})
			npc := &world.NpcInfo{
				ID:     7631,
				NpcID:  45000,
				Impl:   "L1Monster",
				Name:   tt.name + "-npc",
				X:      101,
				Y:      100,
				MapID:  900,
				ShowID: 70,
				Level:  1,
				MR:     0,
			}
			ws.AddNpc(npc)

			s := newSkillTestSystem(t, ws)
			s.executeNpcDebuffSkill(caster.Session, caster, &data.SkillInfo{
				SkillID:      tt.skillID,
				Target:       "buff",
				BuffDuration: 192,
				ActionID:     19,
				CastGfx:      tt.gfxID,
				Ranged:       3,
			}, npc)

			if got := npc.ActiveDebuffs[tt.skillID]; got != 960 {
				t.Fatalf("yiwei %d 對 NPC 會 setSkillEffect 192 秒，got=%d want=960", tt.skillID, got)
			}
			if tt.skillID == 174 && strikerGaleRangedDamageToNpc(npc, 100) != 110 {
				t.Fatal("精準射擊套到 NPC 後，遠程傷害應套用 1.1 倍")
			}

			samePackets := drainSkillTestPackets(sameShow.Session)
			if !hasActionGfxPacket(samePackets, caster.CharID, 19) {
				t.Fatal("同 ShowID 觀察者應收到 NPC debuff 施法動作")
			}
			if !hasSkillEffectPacket(samePackets, npc.ID, tt.gfxID) {
				t.Fatalf("同 ShowID 觀察者應收到 NPC 目標特效 %d，packets=%v", tt.gfxID, samePackets)
			}
			if hasSkillEffectPacket(drainSkillTestPackets(otherShow.Session), npc.ID, tt.gfxID) {
				t.Fatal("不同 ShowID 觀察者不應收到 NPC 目標特效")
			}
		})
	}
}
