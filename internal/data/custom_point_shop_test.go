package data

import (
	"context"
	"errors"
	"testing"
)

func TestCustomPointShopReloadReplacesWholeCatalogueAndPreservesFailedSnapshot(t *testing.T) {
	shops := []CustomPointShop{{ID: 1, NpcID: 100, Name: "shop", Enabled: true}}
	items := []CustomPointShopItem{
		{ID: 3, ShopID: 1, ItemID: 40010, Currency: "MP", Price: 10, PackCount: 1, SortOrder: 5, Enabled: true},
		{ID: 2, ShopID: 1, ItemID: 40011, Currency: "MP", Price: 10, PackCount: 1, SortOrder: 0, Enabled: true},
		{ID: 1, ShopID: 1, ItemID: 40012, Currency: "MP", Price: 10, PackCount: 1, SortOrder: 0, Enabled: true},
		{ID: 4, ShopID: 1, ItemID: 40013, Currency: "MP", Price: 10, PackCount: 1, Enabled: false},
	}
	var loadErr error
	cache := NewCustomPointShopCache(func(context.Context) ([]CustomPointShop, []CustomPointShopItem, error) {
		return shops, items, loadErr
	})
	ctx := context.Background()
	s, i, err := cache.Reload(ctx)
	if err != nil || s != 1 || i != 3 {
		t.Fatalf("reload = %d/%d %v", s, i, err)
	}
	got := cache.Items(1)
	if len(got) != 3 || got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 {
		t.Fatalf("bad ordering/filter: %+v", got)
	}
	got[0].Price = -1
	if cache.Items(1)[0].Price != 10 {
		t.Fatal("caller changed immutable catalogue")
	}
	shops = []CustomPointShop{{ID: 2, NpcID: 200, Name: "replacement"}}
	loadErr = errors.New("database unavailable")
	if _, _, err := cache.Reload(ctx); err == nil {
		t.Fatal("reload swallowed failure")
	}
	if _, ok := cache.ShopByNPC(100); !ok {
		t.Fatal("failed load discarded old catalogue")
	}
	loadErr = nil
	// Old item still references shop 1. A corrupt refresh must retain both old tables.
	if _, _, err := cache.Reload(ctx); err == nil {
		t.Fatal("accepted orphan shop item")
	}
	if _, ok := cache.ShopByNPC(100); !ok {
		t.Fatal("validation failure discarded old catalogue")
	}
	items = nil
	if s, i, err := cache.Reload(ctx); err != nil || s != 1 || i != 0 {
		t.Fatalf("replacement failed: %d/%d %v", s, i, err)
	}
	if _, ok := cache.ShopByNPC(100); ok || len(cache.Items(1)) != 0 {
		t.Fatal("refresh retained removed entries")
	}
	if shop, ok := cache.ShopByNPC(200); !ok || shop.Enabled {
		t.Fatal("disabled shop definition lost")
	}
}
