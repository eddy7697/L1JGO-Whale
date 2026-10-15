package handler

import (
	stdnet "net"
	"testing"

	"github.com/l1jgo/server/internal/data"
	gonet "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type cookingNpcServiceStub struct {
	consumedObjectID int32
	consumedCount    int32
}

func (s *cookingNpcServiceStub) NpcFullHeal(*gonet.Session, *world.PlayerInfo, int32) {}
func (s *cookingNpcServiceStub) NpcWeaponEnchant(*gonet.Session, *world.PlayerInfo)   {}
func (s *cookingNpcServiceStub) NpcArmorEnchant(*gonet.Session, *world.PlayerInfo)    {}
func (s *cookingNpcServiceStub) NpcPoly(*gonet.Session, *world.PlayerInfo, int32)     {}
func (s *cookingNpcServiceStub) NpcTeleportWithCost(*gonet.Session, *world.PlayerInfo, *data.TeleportDest, int32) {
}
func (s *cookingNpcServiceStub) NpcUpgrade(*gonet.Session, *world.PlayerInfo, *data.ItemUpgrade) {}
func (s *cookingNpcServiceStub) ConsumeAdena(*gonet.Session, *world.PlayerInfo, int32) bool {
	return true
}
func (s *cookingNpcServiceStub) RepairWeapon(*gonet.Session, *world.PlayerInfo, *world.InvItem, int32) bool {
	return true
}
func (s *cookingNpcServiceStub) ConsumeItem(_ *gonet.Session, _ *world.PlayerInfo, objectID int32, count int32) bool {
	s.consumedObjectID = objectID
	s.consumedCount = count
	return true
}
func (s *cookingNpcServiceStub) Refine(*gonet.Session, *world.PlayerInfo, *world.InvItem, int32, int32) {
}
func (s *cookingNpcServiceStub) FireSmithCraft(*gonet.Session, *world.PlayerInfo, *data.FireSmithRecipe, int32, int32) {
}

type cookingItemCreateStub struct {
	itemID int32
	count  int32
}

func (s *cookingItemCreateStub) GiveItem(_ *gonet.Session, _ *world.PlayerInfo, itemID, count int32) (*world.InvItem, bool) {
	s.itemID = itemID
	s.count = count
	return &world.InvItem{ItemID: itemID, Count: count}, true
}

func newCookingTestSession(t *testing.T) *gonet.Session {
	t.Helper()
	server, client := stdnet.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	return gonet.NewSession(server, 1, 16, 16, 0, zap.NewNop())
}
