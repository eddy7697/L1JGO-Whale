package handler

import (
	"testing"

	"github.com/l1jgo/server/internal/data"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func TestHandleUseItemRoutesDirectPolyScrollToPolymorphManager(t *testing.T) {
	items, err := data.LoadItemTable(
		"../../data/yaml/weapon_list.yaml",
		"../../data/yaml/armor_list.yaml",
		"../../data/yaml/etcitem_list.yaml",
	)
	if err != nil {
		t.Fatalf("讀取物品資料失敗: %v", err)
	}

	ws := world.NewState()
	sess := &l1net.Session{ID: 1}
	player := &world.PlayerInfo{
		SessionID: sess.ID,
		Session:   sess,
		CharID:    1001,
		Name:      "direct-poly",
		HP:        100,
		Level:     50,
		Inv:       world.NewInventory(),
	}
	scroll := player.Inv.AddItemWithID(7001, 49220, 1, "妖魔密使變形卷軸", 472, 1000, true, 1)
	ws.AddPlayer(player)
	poly := &capturePolymorphManager{}
	deps := &Deps{World: ws, Items: items, Polymorph: poly, Log: zap.NewNop()}

	HandleUseItem(sess, useItemReader(scroll.ObjectID), deps)

	if poly.directPolyItemID != 49220 {
		t.Fatalf("直接變身卷軸應委派 Polymorph.UseDirectPolyScroll，got=%d", poly.directPolyItemID)
	}
}

func useItemReader(objectID int32) *packet.Reader {
	w := packet.NewWriterWithOpcode(packet.C_OPCODE_USE_ITEM)
	w.WriteD(objectID)
	return packet.NewReader(w.Bytes())
}
