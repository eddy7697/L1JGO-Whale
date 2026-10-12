package data

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
)

// CustomPointShop and CustomPointShopItem mirror the Java CP catalogue. They
// are separate from ShopCn, which charges inventory currency instead of points.
type CustomPointShop struct {
	ID, NpcID int32
	Name      string
	Enabled   bool
}

type CustomPointShopItem struct {
	ID, ShopID, ItemID                  int32
	Currency                            string
	Price                               int64
	PackCount, DailyLimit, AccountLimit int32
	SortOrder                           int32
	Enabled                             bool
}

type CustomPointShopLoader func(context.Context) ([]CustomPointShop, []CustomPointShopItem, error)

type customPointShopSnapshot struct {
	shops map[int32]CustomPointShop
	byNPC map[int32]int32
	items map[int32][]CustomPointShopItem
}

// CustomPointShopCache swaps a complete catalogue only after both tables load.
// SQL or validation failures retain the last successfully loaded catalogue.
type CustomPointShopCache struct {
	load    CustomPointShopLoader
	current atomic.Pointer[customPointShopSnapshot]
}

func NewCustomPointShopCache(load CustomPointShopLoader) *CustomPointShopCache {
	return &CustomPointShopCache{load: load}
}

func (c *CustomPointShopCache) Reload(ctx context.Context) (int, int, error) {
	shops, items, err := c.load(ctx)
	if err != nil {
		return 0, 0, err
	}
	next := &customPointShopSnapshot{shops: make(map[int32]CustomPointShop), byNPC: make(map[int32]int32), items: make(map[int32][]CustomPointShopItem)}
	for _, shop := range shops {
		if shop.ID <= 0 || shop.NpcID <= 0 || shop.Name == "" {
			return 0, 0, fmt.Errorf("invalid custom point shop %d", shop.ID)
		}
		if _, exists := next.shops[shop.ID]; exists {
			return 0, 0, fmt.Errorf("duplicate custom point shop %d", shop.ID)
		}
		if _, exists := next.byNPC[shop.NpcID]; exists {
			return 0, 0, fmt.Errorf("duplicate custom point shop NPC %d", shop.NpcID)
		}
		next.shops[shop.ID], next.byNPC[shop.NpcID] = shop, shop.ID
	}
	count := 0
	seen := make(map[int32]bool)
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		if _, exists := next.shops[item.ShopID]; !exists || seen[item.ID] || item.ID <= 0 || item.ItemID <= 0 || item.Price <= 0 || item.Currency == "" || item.PackCount <= 0 || item.DailyLimit < 0 || item.AccountLimit < 0 {
			return 0, 0, fmt.Errorf("invalid custom point shop item %d", item.ID)
		}
		seen[item.ID] = true
		next.items[item.ShopID] = append(next.items[item.ShopID], item)
		count++
	}
	for _, group := range next.items {
		sort.Slice(group, func(i, j int) bool {
			if group[i].SortOrder == group[j].SortOrder {
				return group[i].ID < group[j].ID
			}
			return group[i].SortOrder < group[j].SortOrder
		})
	}
	c.current.Store(next)
	return len(shops), count, nil
}

func (c *CustomPointShopCache) ShopByNPC(npcID int32) (CustomPointShop, bool) {
	s := c.current.Load()
	if s == nil {
		return CustomPointShop{}, false
	}
	id, ok := s.byNPC[npcID]
	return s.shops[id], ok
}

func (c *CustomPointShopCache) Items(shopID int32) []CustomPointShopItem {
	s := c.current.Load()
	if s == nil {
		return nil
	}
	return append([]CustomPointShopItem(nil), s.items[shopID]...)
}
