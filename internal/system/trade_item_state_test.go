package system

import (
	stdnet "net"
	"testing"

	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func newTradeTestSession(t *testing.T, id uint64) *l1net.Session {
	t.Helper()
	client, server := stdnet.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
	})
	sess := l1net.NewSession(server, id, 16, 16, 0, zap.NewNop())
	t.Cleanup(sess.Close)
	return sess
}

func newTradeTestPlayer(t *testing.T, sessionID uint64, charID int32, name string) *world.PlayerInfo {
	t.Helper()
	return &world.PlayerInfo{
		SessionID: sessionID,
		Session:   newTradeTestSession(t, sessionID),
		CharID:    charID,
		Name:      name,
		X:         int32(10 + sessionID),
		Y:         10,
		MapID:     4,
		Str:       20,
		Con:       20,
		Inv:       world.NewInventory(),
	}
}

func addTradeStateItem(player *world.PlayerInfo) *world.InvItem {
	item := player.Inv.AddItem(1003, 1, "測試魔杖", 103, 10, false, 2)
	item.UseType = 17
	item.ChargeCount = 7
	item.Durability = 3
	item.AttrEnchantKind = 2
	item.AttrEnchantLevel = 4
	item.Identified = false
	item.InnKeyID = 12345
	item.InnNpcID = 70012
	item.InnHall = true
	item.InnDueTime = 99_999
	item.Accessory = world.AccessoryBonus{AddHP: 2, MDef: 1}
	return item
}

func assertTradeItemStatePreserved(t *testing.T, item *world.InvItem, objectID int32) {
	t.Helper()
	if item == nil {
		t.Fatal("應保留交易物品")
	}
	if item.ObjectID != objectID {
		t.Fatalf("ObjectID 未保留，got %d want %d", item.ObjectID, objectID)
	}
	if item.ChargeCount != 7 ||
		item.Durability != 3 ||
		item.AttrEnchantKind != 2 ||
		item.AttrEnchantLevel != 4 ||
		item.Identified != false ||
		item.InnKeyID != 12345 ||
		item.InnNpcID != 70012 ||
		item.InnHall != true ||
		item.InnDueTime != 99_999 ||
		item.Accessory != (world.AccessoryBonus{AddHP: 2, MDef: 1}) {
		t.Fatalf("交易後物品狀態未完整保留: %+v", item)
	}
}

func newTradeStateSystem(t *testing.T, p1, p2 *world.PlayerInfo) *TradeSystem {
	t.Helper()
	ws := world.NewState()
	ws.AddPlayer(p1)
	ws.AddPlayer(p2)
	return NewTradeSystem(&handler.Deps{
		World: ws,
		Items: testItemTable(t),
		Log:   zap.NewNop(),
	})
}

func openTradeForTest(p1, p2 *world.PlayerInfo) {
	p1.TradePartnerID = p2.CharID
	p1.TradeWindowOpen = true
	p2.TradePartnerID = p1.CharID
	p2.TradeWindowOpen = true
}

func TestTradeCompletePreservesItemState(t *testing.T) {
	sender := newTradeTestPlayer(t, 1, 100, "交易者甲")
	receiver := newTradeTestPlayer(t, 2, 200, "交易者乙")
	item := addTradeStateItem(sender)
	objectID := item.ObjectID
	sys := newTradeStateSystem(t, sender, receiver)
	openTradeForTest(sender, receiver)

	sys.AddItem(sender.Session, sender, objectID, 1)
	sys.Accept(sender.Session, sender)
	sys.Accept(receiver.Session, receiver)

	if sender.Inv.FindByObjectID(objectID) != nil {
		t.Fatal("交易完成後來源背包不應保留原物品")
	}
	assertTradeItemStatePreserved(t, receiver.Inv.FindByObjectID(objectID), objectID)
}

func TestTradeCancelRestoresItemState(t *testing.T) {
	player := newTradeTestPlayer(t, 1, 100, "交易者甲")
	partner := newTradeTestPlayer(t, 2, 200, "交易者乙")
	item := addTradeStateItem(player)
	objectID := item.ObjectID
	sys := newTradeStateSystem(t, player, partner)
	openTradeForTest(player, partner)

	sys.AddItem(player.Session, player, objectID, 1)
	sys.Cancel(player)

	assertTradeItemStatePreserved(t, player.Inv.FindByObjectID(objectID), objectID)
}

func TestDisconnectTradeRestorePreservesItemState(t *testing.T) {
	player := newTradeTestPlayer(t, 1, 100, "交易者甲")
	item := addTradeStateItem(player)
	objectID := item.ObjectID
	tradeCopy := *item
	tradeCopy.Count = 1
	player.TradeItems = []*world.InvItem{&tradeCopy}
	player.Inv.RemoveItem(objectID, 1)

	restoreTradeItemsOnDisconnect(player)

	assertTradeItemStatePreserved(t, player.Inv.FindByObjectID(objectID), objectID)
}

func TestBuildTradeWALEntriesUsesObjectIDForItemTransfer(t *testing.T) {
	sender := newTradeTestPlayer(t, 1, 100, "交易者甲")
	receiver := newTradeTestPlayer(t, 2, 200, "交易者乙")
	item := addTradeStateItem(sender)
	sender.TradeItems = []*world.InvItem{item}
	sender.TradeGold = 30

	entries := buildTradeWALEntries(sender, receiver)
	if len(entries) != 2 {
		t.Fatalf("應產生物品與金幣兩筆 WAL，got %d", len(entries))
	}

	itemEntry := entries[0]
	if itemEntry.ItemID != item.ObjectID {
		t.Fatalf("物品 WAL 應記錄 ObjectID，got %d want %d", itemEntry.ItemID, item.ObjectID)
	}
	if itemEntry.GoldAmount != 0 {
		t.Fatalf("物品 WAL 不應帶 GoldAmount，got %d", itemEntry.GoldAmount)
	}

	goldEntry := entries[1]
	if goldEntry.ItemID != world.AdenaItemID {
		t.Fatalf("金幣 WAL 應保留 Adena item_id 供 recovery 判斷，got %d", goldEntry.ItemID)
	}
	if goldEntry.GoldAmount != int64(sender.TradeGold) {
		t.Fatalf("金幣 WAL 金額錯誤，got %d want %d", goldEntry.GoldAmount, sender.TradeGold)
	}
}

func TestTradePartialStackUsesSeparateTradeObjectID(t *testing.T) {
	sender := newTradeTestPlayer(t, 1, 100, "交易者甲")
	receiver := newTradeTestPlayer(t, 2, 200, "交易者乙")
	item := sender.Inv.AddItem(1001, 10, "測試藥水", 101, 10, true, 1)
	sourceObjectID := item.ObjectID
	sys := newTradeStateSystem(t, sender, receiver)
	openTradeForTest(sender, receiver)

	sys.AddItem(sender.Session, sender, sourceObjectID, 3)

	sourceItem := sender.Inv.FindByObjectID(sourceObjectID)
	if sourceItem == nil || sourceItem.Count != 7 {
		t.Fatalf("來源堆疊應保留原 ObjectID 與剩餘數量，got %+v", sourceItem)
	}
	if len(sender.TradeItems) != 1 {
		t.Fatalf("應有一筆交易暫存物，got %d", len(sender.TradeItems))
	}
	tradeItem := sender.TradeItems[0]
	if tradeItem.ObjectID == sourceObjectID {
		t.Fatalf("部分堆疊交易暫存物不可重用來源 ObjectID，got %d", tradeItem.ObjectID)
	}
	if tradeItem.Count != 3 {
		t.Fatalf("交易暫存物數量錯誤，got %d want 3", tradeItem.Count)
	}

	entries := buildTradeWALEntries(sender, receiver)
	if len(entries) != 1 || entries[0].ItemID != item.ItemID || entries[0].Count != tradeItem.Count {
		t.Fatalf("堆疊物品 WAL 應記錄模板 item_id 與轉移數量，entries=%+v itemID=%d count=%d", entries, item.ItemID, tradeItem.Count)
	}
}
