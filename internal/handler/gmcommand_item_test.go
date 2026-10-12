package handler

import (
	"testing"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/data"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type captureGMCommandManager struct {
	called       bool
	itemID       int32
	count        int32
	enchant      int8
	lawfulCalled bool
	lawfulDelta  int32
	lawfulPlayer *world.PlayerInfo
}

func (m *captureGMCommandManager) SetLevel(_ *l1net.Session, _ *world.PlayerInfo, _ int) {}
func (m *captureGMCommandManager) SetHP(_ *l1net.Session, _ *world.PlayerInfo, _ int)    {}
func (m *captureGMCommandManager) SetMP(_ *l1net.Session, _ *world.PlayerInfo, _ int)    {}
func (m *captureGMCommandManager) FullHeal(_ *l1net.Session, _ *world.PlayerInfo)        {}
func (m *captureGMCommandManager) SetStat(_ *l1net.Session, _ *world.PlayerInfo, _ string, _ int16) {
}
func (m *captureGMCommandManager) GiveItem(_ *l1net.Session, _ *world.PlayerInfo, itemID, count int32, enchant int8) {
	m.called = true
	m.itemID = itemID
	m.count = count
	m.enchant = enchant
}
func (m *captureGMCommandManager) GiveGold(_ *l1net.Session, _ *world.PlayerInfo, _ int32) {}
func (m *captureGMCommandManager) AdjustLawful(_ *l1net.Session, player *world.PlayerInfo, delta int32) {
	m.lawfulCalled = true
	m.lawfulDelta = delta
	m.lawfulPlayer = player
}
func (m *captureGMCommandManager) ApplyPoison(_ *world.PlayerInfo, _ byte) bool { return false }
func (m *captureGMCommandManager) BreakWeapon(_ *world.PlayerInfo, _ int8) (string, bool) {
	return "", false
}

func TestGMItemParsesEnchantBeforeCountForWeapon(t *testing.T) {
	capture := &captureGMCommandManager{}
	sess, player, deps := newGMItemCommandTestContext(t, capture)

	gmItem(sess, player, []string{"31", "+7", "3"}, deps)

	if !capture.called {
		t.Fatal(".item 武器 +強化 數量 應委派 GiveItem")
	}
	if capture.itemID != 31 || capture.count != 3 || capture.enchant != 7 {
		t.Fatalf("解析結果錯誤，item=%d count=%d enchant=%d", capture.itemID, capture.count, capture.enchant)
	}
}

func TestGMItemParsesCountOnlyForStackableItem(t *testing.T) {
	capture := &captureGMCommandManager{}
	sess, player, deps := newGMItemCommandTestContext(t, capture)

	gmItem(sess, player, []string{"40010", "5"}, deps)

	if !capture.called {
		t.Fatal(".item 物品 數量 應委派 GiveItem")
	}
	if capture.itemID != 40010 || capture.count != 5 || capture.enchant != 0 {
		t.Fatalf("解析結果錯誤，item=%d count=%d enchant=%d", capture.itemID, capture.count, capture.enchant)
	}
}

func TestGMItemRejectsEnchantForEtcItem(t *testing.T) {
	capture := &captureGMCommandManager{}
	sess, player, deps := newGMItemCommandTestContext(t, capture)

	gmItem(sess, player, []string{"40010", "+7", "5"}, deps)

	if capture.called {
		t.Fatal("道具不是武器/防具時不得接受 +強化")
	}
}

func TestGMItemKeepsLegacyCountEnchantOrder(t *testing.T) {
	capture := &captureGMCommandManager{}
	sess, player, deps := newGMItemCommandTestContext(t, capture)

	gmItem(sess, player, []string{"31", "2", "6"}, deps)

	if !capture.called {
		t.Fatal("舊格式 .item 物品 數量 強化 應維持可用")
	}
	if capture.itemID != 31 || capture.count != 2 || capture.enchant != 6 {
		t.Fatalf("舊格式解析結果錯誤，item=%d count=%d enchant=%d", capture.itemID, capture.count, capture.enchant)
	}
}

func newGMItemCommandTestContext(t *testing.T, gm GMCommandManager) (*l1net.Session, *world.PlayerInfo, *Deps) {
	t.Helper()
	sess := newHandlerTestSession(t, 1)
	player := &world.PlayerInfo{
		SessionID:   sess.ID,
		Session:     sess,
		CharID:      1001,
		Name:        "gm",
		AccessLevel: 200,
		Inv:         world.NewInventory(),
	}
	deps := &Deps{
		Items:  mustLoadGMItemCommandItems(t),
		GMCmd:  gm,
		Config: &config.Config{},
		Log:    zap.NewNop(),
	}
	return sess, player, deps
}

func mustLoadGMItemCommandItems(t *testing.T) *data.ItemTable {
	t.Helper()
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatalf("載入物品表失敗: %v", err)
	}
	return items
}
