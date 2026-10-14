package world

import "sync/atomic"

// groundItemIDCounter generates unique object IDs for ground items.
// Starts at 700_000_000 to avoid collision with char/NPC/inventory IDs.
var groundItemIDCounter atomic.Int32

func init() {
	groundItemIDCounter.Store(700_000_000)
}

// NextGroundItemID returns a unique object ID for a ground item.
func NextGroundItemID() int32 {
	return groundItemIDCounter.Add(1)
}

// GroundItem represents an item on the ground that players can see and pick up.
// Not persisted to DB — exists only in memory.
type GroundItem struct {
	ID         int32 // unique ground object ID
	ItemID     int32 // template ID
	Count      int32 // stack count
	EnchantLvl int8  // enchant level (signed: cursed scrolls can go negative)
	Item       *InvItem
	Name       string // display name
	GrdGfx     int32  // ground visual GFX ID
	X          int32
	Y          int32
	MapID      int16
	ShowID     int32
	OwnerID    int32 // 擁有者 CharID（Java: L1ItemInstance.itemOwnerId，0 = 任何人可撿）
	OwnerTicks int   // 擁有者保護剩餘 tick（Java: L1ItemOwnerTimer 10 秒）；歸零時清除 OwnerID
	TTL        int   // ticks remaining until auto-delete (0 = permanent)
	NoExpire   bool  // true = 不自動消失（血盟小屋內物品）
}
