package system

import (
	stdnet "net"
	"testing"

	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func newItemGroundTestSession(t *testing.T) *l1net.Session {
	t.Helper()
	client, server := stdnet.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
	})
	sess := l1net.NewSession(server, 1, 16, 16, 0, zap.NewNop())
	t.Cleanup(sess.Close)
	return sess
}

func TestItemGroundDropAndPickupPreservesItemState(t *testing.T) {
	ws := world.NewState()
	player := &world.PlayerInfo{
		Session: newItemGroundTestSession(t),
		CharID:  100,
		Name:    "測試玩家",
		X:       10,
		Y:       10,
		MapID:   4,
		Str:     20,
		Con:     20,
		Inv:     world.NewInventory(),
	}
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

	sys := NewItemGroundSystem(&handler.Deps{
		World: ws,
		Items: testItemTable(t),
		Log:   zap.NewNop(),
	})

	sys.DropItem(player.Session, player, item.ObjectID, 1)
	ground := ws.GetNearbyGroundItems(player.X, player.Y, player.MapID)
	if len(ground) != 1 {
		t.Fatalf("掉落後地面物品數量錯誤，got %d want 1", len(ground))
	}

	sys.PickupItem(player.Session, player, ground[0].ID)
	picked := player.Inv.FindByItemID(1003)
	if picked == nil {
		t.Fatal("撿取後背包應有原物品")
	}

	if picked.ChargeCount != 7 ||
		picked.Durability != 3 ||
		picked.AttrEnchantKind != 2 ||
		picked.AttrEnchantLevel != 4 ||
		picked.Identified != false ||
		picked.InnKeyID != 12345 ||
		picked.InnNpcID != 70012 ||
		picked.InnHall != true ||
		picked.InnDueTime != 99_999 ||
		picked.Accessory != (world.AccessoryBonus{AddHP: 2, MDef: 1}) {
		t.Fatalf("撿取後物品狀態未完整保留: %+v", picked)
	}
}
