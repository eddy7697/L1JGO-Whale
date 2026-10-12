package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type gmAdvPolyTestStore struct {
	charID, polyID int32
	count          int32
	resets         int
	err            error
}

func (s *gmAdvPolyTestStore) Unlock(_ context.Context, charID, polyID int32) (int32, error) {
	s.charID, s.polyID = charID, polyID
	if s.err != nil {
		return 0, s.err
	}
	s.count++
	return s.count, nil
}

func (s *gmAdvPolyTestStore) Reset(_ context.Context, charID int32) error {
	s.charID = charID
	if s.err != nil {
		return s.err
	}
	s.resets++
	s.count = 0
	return nil
}

type gmAdvPolyTestGiver struct {
	captureGMCommandManager
	fail bool
}

func (m *gmAdvPolyTestGiver) GiveItem(_ *net.Session, player *world.PlayerInfo, itemID, count int32, enchant int8) {
	m.called, m.itemID, m.count, m.enchant = true, itemID, count, enchant
	if !m.fail {
		player.Inv.AddItem(itemID, count, "card", 0, 0, false, 1)
	}
}

func TestGMAdvPolyGiveAcceptsCardOrPolyAndOnlyGrantsOneCard(t *testing.T) {
	items := mustLoadGMItemCommandItems(t)
	for _, id := range []string{"13715", "600123"} {
		t.Run(id, func(t *testing.T) {
			gmSess := newHandlerTestSession(t, 1)
			targetSess := newHandlerTestSession(t, 2)
			ws := world.NewState()
			target := &world.PlayerInfo{CharID: 12, Name: "Target", SessionID: 2, Session: targetSess, Inv: world.NewInventory(), PolyID: 13719}
			ws.AddPlayer(target)
			giver := &gmAdvPolyTestGiver{}
			deps := &Deps{World: ws, Items: items, GMCmd: giver}
			gmAdminAdvPoly(gmSess, "advpolygive", []string{"Target", id}, deps)
			if !giver.called || giver.itemID != 600123 || giver.count != 1 || giver.enchant != 0 || gmAdvPolyCardCount(target, 600123) != 1 {
				t.Fatalf("發卡未對齊 Java: %+v", giver)
			}
			if target.PolyID != 13719 {
				t.Fatal("發卡不應立即更換玩家變身")
			}
		})
	}
}

func TestGMAdvPolyRejectsUnknownCardAndFullInventory(t *testing.T) {
	gmSess := newHandlerTestSession(t, 1)
	ws := world.NewState()
	target := &world.PlayerInfo{CharID: 12, Name: "Target", SessionID: 2, Session: newHandlerTestSession(t, 2), Inv: world.NewInventory()}
	ws.AddPlayer(target)
	giver := &gmAdvPolyTestGiver{}
	deps := &Deps{World: ws, Items: mustLoadGMItemCommandItems(t), GMCmd: giver}
	for _, id := range []string{"600100", "600151", "999999", "2147483648", "-1"} {
		gmAdminAdvPoly(gmSess, "advpolygive", []string{"Target", id}, deps)
		if giver.called {
			t.Fatalf("非法 ID %s 不應發卡", id)
		}
	}
	for i := 0; i < world.MaxInventorySize; i++ {
		target.Inv.AddItem(31, 1, "sword", 0, 0, false, 1)
	}
	gmAdminAdvPoly(gmSess, "advpolygive", []string{"Target", "13715"}, deps)
	if giver.called {
		t.Fatal("背包已滿不應發卡")
	}
}

func TestGMAdvPolyUnlockAndResetTouchOnlyCollection(t *testing.T) {
	gmSess := newHandlerTestSession(t, 1)
	ws := world.NewState()
	target := &world.PlayerInfo{CharID: 12, Name: "Target", SessionID: 2, Session: newHandlerTestSession(t, 2), Inv: world.NewInventory(), PolyID: 13715}
	target.Inv.AddItem(600123, 1, "card", 0, 0, false, 1)
	ws.AddPlayer(target)
	store := &gmAdvPolyTestStore{}
	deps := &Deps{World: ws, AdvancedPoly: store, Log: zap.NewNop()}
	for i := 0; i < 2; i++ {
		gmAdminAdvPoly(gmSess, "advpolyunlock", []string{"Target", "13715"}, deps)
	}
	if store.count != 2 || store.charID != target.CharID || store.polyID != 13715 {
		t.Fatalf("解鎖必須按角色累加: %+v", store)
	}
	gmAdminAdvPoly(gmSess, "advpolyunlock", []string{"Target", "600123"}, deps)
	if store.count != 2 {
		t.Fatal("unlock 只接受 polyID，不接受卡片 ID")
	}
	gmAdminAdvPoly(gmSess, "advpolyreset", []string{"Target"}, deps)
	if store.resets != 1 || store.count != 0 || target.PolyID != 13715 || gmAdvPolyCardCount(target, 600123) != 1 {
		t.Fatal("重置應僅刪除收藏，保留變身與卡片")
	}
	store.err = errors.New("database offline")
	gmAdminAdvPoly(gmSess, "advpolyunlock", []string{"Target", "13715"}, deps)
	if store.count != 0 {
		t.Fatal("資料庫失敗不得視為解鎖成功")
	}
}
