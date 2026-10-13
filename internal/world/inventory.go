package world

import (
	"math"
	"math/rand"
	"sync/atomic"
)

// RandInt returns a random int in [0, n). Safe to call from game loop goroutine.
func RandInt(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

const (
	MaxInventorySize = 255
	AdenaItemID      = 40308
)

// itemObjIDCounter generates unique item object IDs.
// Starts at 500_000_000 to avoid collision with char IDs and NPC IDs.
var itemObjIDCounter atomic.Int32

func init() {
	itemObjIDCounter.Store(500_000_000)
}

// NextItemObjID returns a unique object ID for an item instance.
func NextItemObjID() int32 {
	return itemObjIDCounter.Add(1)
}

// SetItemObjIDStart sets the counter start value.
// Called on startup with max(persisted_max_obj_id, 500_000_000) to avoid collisions.
func SetItemObjIDStart(v int32) {
	itemObjIDCounter.Store(v)
}

// InvItem represents a single item instance in a player's inventory.
type InvItem struct {
	ObjectID    int32  // unique per instance
	ItemID      int32  // template ID
	Name        string // display name
	InvGfx      int32  // inventory graphic ID
	Count       int32  // stack count (1 for non-stackable)
	Identified  bool
	EnchantLvl  int8
	Bless       byte // 0=normal, 1=blessed, 2=cursed, >=128=sealed
	Stackable   bool
	Weight      int32 // per-unit weight
	UseType     byte
	ChargeCount int16 // 魔杖充能次數（0=無限制或不適用；>0=剩餘使用次數）
	Equipped    bool  // true if currently worn/wielded
	LastUsedAt  int64 // delay_effect 專用上次使用時間（Unix 秒，0=未使用）
	NowLighting bool  // 燈具開關，登入時預設熄滅（Java isNowLighting）
	LightFuel   int32 // 燈具剩餘燃料，登入重新填滿，與 Eddy 一致

	// Weapon durability: 0 = perfect, higher = more damaged (range 0-127).
	// Effective enchant = EnchantLvl - Durability (Java: L1Attack line 328).
	// Repair NPC sets to 0; combat damage increments by 1 with probability check.
	Durability int8

	// NPC enchant spell temporary bonuses (item-level, not character-level).
	// Java: L1ItemInstance.setSkillWeaponEnchant / setSkillArmorEnchant
	DmgByMagic     int16 // +damage from weapon enchant magic
	HitByMagic     int16 // +hit from weapon enchant magic
	DmgMagicExpiry int   // ticks remaining (0 = no effect)
	AcByMagic      int16 // AC bonus from BLESSED_ARMOR (skill 21), typically 3 (applied as -3 AC)
	AcMagicExpiry  int   // ticks remaining (0 = no effect)

	// 元素屬性強化（Java: L1ItemInstance.attrEnchantKind / attrEnchantLevel）
	// Kind: 0=無, 1=地, 2=火, 4=水, 8=風
	// Level: 0-5（強化階段）
	AttrEnchantKind  int8
	AttrEnchantLevel int8

	// 旅館鑰匙額外欄位（Java: L1ItemInstance.keyId / innNpcId / isHall / dueTime）
	InnKeyID   int32 // 鑰匙 ID（= 物品 ObjectID，用於匹配房間）
	InnNpcID   int32 // 旅館 NPC 模板 ID
	InnHall    bool  // 是否為會議室鑰匙
	InnDueTime int64 // 租約到期時間（Unix 秒，0=非旅館鑰匙）
}

// Inventory holds a player's in-memory item list.
// Accessed only from the game loop goroutine.
type Inventory struct {
	Items []*InvItem
}

// NewInventory creates an empty inventory.
func NewInventory() *Inventory {
	return &Inventory{
		Items: make([]*InvItem, 0, 16),
	}
}

// FindByItemID returns the first item matching the template ID (for stackable items).
func (inv *Inventory) FindByItemID(itemID int32) *InvItem {
	for _, it := range inv.Items {
		if it.ItemID == itemID {
			return it
		}
	}
	return nil
}

// FindByObjectID returns the item with the given object ID.
func (inv *Inventory) FindByObjectID(objectID int32) *InvItem {
	for _, it := range inv.Items {
		if it.ObjectID == objectID {
			return it
		}
	}
	return nil
}

// Size returns the number of item slots used.
func (inv *Inventory) Size() int {
	return len(inv.Items)
}

// IsFull returns true if inventory is at max capacity.
func (inv *Inventory) IsFull() bool {
	return len(inv.Items) >= MaxInventorySize
}

// AddItem adds or stacks an item. Returns the affected item (new or existing).
// Does NOT send packets — caller is responsible.
func (inv *Inventory) AddItem(itemID int32, count int32, name string, invGfx int32, weight int32, stackable bool, bless byte) *InvItem {
	return inv.AddItemWithID(0, itemID, count, name, invGfx, weight, stackable, bless)
}

// AddItemWithID adds an item with a specific ObjectID (used for DB reload to preserve shortcut references).
// If objID is 0, a new ObjectID is generated.
func (inv *Inventory) AddItemWithID(objID int32, itemID int32, count int32, name string, invGfx int32, weight int32, stackable bool, bless byte) *InvItem {
	if stackable {
		existing := inv.FindByItemID(itemID)
		if existing != nil {
			existing.Count += count
			return existing
		}
	}

	if objID == 0 {
		objID = NextItemObjID()
	}

	item := &InvItem{
		ObjectID:   objID,
		ItemID:     itemID,
		Name:       name,
		InvGfx:     invGfx,
		Count:      count,
		Identified: true,
		Stackable:  stackable,
		Weight:     weight,
		Bless:      bless,
		LightFuel:  ItemLightFuel(itemID),
	}
	inv.Items = append(inv.Items, item)
	return item
}

// RemoveItem removes count from a stackable item or removes the item entirely.
// Returns true if the item was fully removed (slot freed), false if just decremented.
func (inv *Inventory) RemoveItem(objectID int32, count int32) (removed bool) {
	for i, it := range inv.Items {
		if it.ObjectID == objectID {
			if it.Stackable && it.Count > count {
				it.Count -= count
				return false
			}
			// Remove slot entirely
			inv.Items = append(inv.Items[:i], inv.Items[i+1:]...)
			return true
		}
	}
	return false
}

// GetAdena returns the current adena count.
func (inv *Inventory) GetAdena() int32 {
	item := inv.FindByItemID(AdenaItemID)
	if item == nil {
		return 0
	}
	return item.Count
}

// WeightBarMax is the client weight-bar scale used by the 3.80C client
// (Java: L1PcInventory.calcWeight242 / S_PacketBox.WEIGHT).
const WeightBarMax = 242

// StackWeight returns the display weight of a stack of `count` units whose
// template weight is `unitWeight` (template weight is in 1/1000 display units).
// Java: L1ItemInstance.getWeight() = max(count * templateWeight / 1000, 1),
// or 0 when the template weight is 0.
// The product is computed in int64 so large stacks (e.g. 500,000 × 5000) do not
// wrap around int32 and collapse to the 1-unit floor.
func StackWeight(unitWeight, count int32) int32 {
	if unitWeight == 0 {
		return 0
	}
	w := int64(count) * int64(unitWeight) / 1000
	if w < 1 {
		return 1
	}
	if w > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(w)
}

// AddWeight returns the display weight that adding `count` units of a template
// with `unitWeight` would contribute, as used by the acquisition check.
// Java L1PcInventory.checkAddItem() line 110: `item.getItem().getWeight() * count / 1000`
// (no 1-unit floor). Java multiplies in int, and line 111 rejects a negative
// (overflowed) product with WEIGHT_OVER; we detect the overflow explicitly and
// return -1 so IsOverWeight refuses it the same way.
func AddWeight(unitWeight, count int32) int64 {
	product := int64(unitWeight) * int64(count)
	if product > math.MaxInt32 || product < math.MinInt32 {
		return -1
	}
	return product / 1000
}

// HasEquipped reports whether an item with the given template ID is currently worn.
// Java: L1PcInventory.checkEquipped(itemId).
func (inv *Inventory) HasEquipped(itemID int32) bool {
	if inv == nil {
		return false
	}
	for _, it := range inv.Items {
		if it.Equipped && it.ItemID == itemID {
			return true
		}
	}
	return false
}

// StackWeight returns this item's display weight (Java: L1ItemInstance.getWeight()).
func (it *InvItem) StackWeight() int32 {
	return StackWeight(it.Weight, it.Count)
}

// TotalWeight returns the total carried weight in display units.
// Java: L1Inventory.getWeight() sums L1ItemInstance.getWeight() over all items.
func (inv *Inventory) TotalWeight() int32 {
	var total int64
	for _, it := range inv.Items {
		total += int64(it.StackWeight())
	}
	if total > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(total)
}

// WeightLimitRate mirrors Java Config.RATE_WEIGHT_LIMIT (L1PcInstance.getMaxWeight line 2962).
// Set from config at startup; 0 makes every weight bar read 0 (L1PcInventory.calcWeight242 line 75-88).
var WeightLimitRate = 1.0

// MaxWeight calculates the base carrying capacity from STR/CON.
// Java L1PcInstance.getMaxWeight() line 2939: 150 * floor(0.6*STR + 0.4*CON + 1),
// where getStr()/getCon() are the current totals including equipment and buffs.
func MaxWeight(str, con int16) int32 {
	return int32(150 * math.Floor(0.6*float64(str)+0.4*float64(con)+1))
}

// Class type constants used by the original-stat weight tables (PlayerInfo.ClassType).
const (
	classPrince       = 0
	classKnight       = 1
	classElf          = 2
	classWizard       = 3
	classDarkElf      = 4
	classDragonKnight = 5
	classIllusionist  = 6
)

// originalStatLikeJava is a fallback for synthetic players without persisted
// creation stats. Real characters load original_str/original_con from the DB;
// those values must not change with equipment, dolls, level bonuses or elixirs.
func originalStatLikeJava(total int16, equip int, buffDelta func(*ActiveBuff) int16, p *PlayerInfo) int {
	base := int(total) - equip
	for _, b := range p.ActiveBuffs {
		if b != nil {
			base -= int(buffDelta(b))
		}
	}
	if base < 0 {
		return 0
	}
	return base
}

// OriginalStrWeightReduction mirrors L1PcInstance.resetOriginalStrWeightReduction() (lines 3750-3797).
func OriginalStrWeightReduction(p *PlayerInfo) int {
	s := int(p.OriginalStr)
	if s <= 0 {
		s = originalStatLikeJava(p.Str, p.EquipBonuses.AddStr, func(b *ActiveBuff) int16 { return b.DeltaStr }, p)
	}
	switch p.ClassType {
	case classPrince:
		switch {
		case s >= 14 && s <= 16:
			return 1
		case s >= 17 && s <= 19:
			return 2
		case s == 20:
			return 3
		}
	case classElf:
		if s >= 16 {
			return 2
		}
	case classDarkElf:
		switch {
		case s >= 13 && s <= 15:
			return 2
		case s >= 16:
			return 3
		}
	case classWizard:
		if s >= 9 {
			return 1
		}
	case classDragonKnight:
		if s >= 16 {
			return 1
		}
	case classIllusionist:
		if s == 18 {
			return 1
		}
	}
	return 0
}

// OriginalConWeightReduction mirrors L1PcInstance.resetOriginalConWeightReduction() (lines 3860-3905).
func OriginalConWeightReduction(p *PlayerInfo) int {
	c := int(p.OriginalCon)
	if c <= 0 {
		c = originalStatLikeJava(p.Con, p.EquipBonuses.AddCon, func(b *ActiveBuff) int16 { return b.DeltaCon }, p)
	}
	switch p.ClassType {
	case classPrince:
		if c >= 11 {
			return 1
		}
	case classKnight:
		if c >= 15 {
			return 1
		}
	case classElf:
		if c >= 15 {
			return 2
		}
	case classDarkElf:
		if c >= 9 {
			return 1
		}
	case classWizard:
		switch {
		case c == 13 || c == 14:
			return 1
		case c >= 15:
			return 2
		}
	case classIllusionist:
		switch c {
		case 17:
			return 1
		case 18:
			return 2
		}
	}
	return 0
}

// PlayerMaxWeight computes the effective carrying capacity exactly like
// Java L1PcInstance.getMaxWeight() (lines 2936-2965), which returns a double:
//
//	max  = 150 * floor(0.6*STR + 0.4*CON + 1)
//	max *= 1 + armorReduction/100 + dollReduction/100 + 0.04*(origStrWR + origConWR)
//	max += 180 if DECREASE_WEIGHT
//	max *= Config.RATE_WEIGHT_LIMIT
//
// WeightBonusPct (yiwei add_weightUP, not in Eddy Java) is folded into the same
// percent multiplier.
func PlayerMaxWeight(p *PlayerInfo) float64 {
	maxWeight := float64(MaxWeight(p.Str, p.Con))

	reduction := 1.0
	reduction += float64(p.EquipBonuses.WeightReduction) / 100 // 防具 weight_reduction（%）
	reduction += float64(p.WeightBonus) / 100                  // L1MagicDoll.getWeightReductionByDoll（%）
	reduction += 0.04 * float64(OriginalStrWeightReduction(p)+OriginalConWeightReduction(p))
	reduction += float64(p.WeightBonusPct) / 100
	maxWeight *= reduction

	if p.HasBuff(14) { // DECREASE_WEIGHT; 218 is JOY_OF_PAIN, not a weight buff.
		maxWeight += 180
	}

	maxWeight *= WeightLimitRate
	return maxWeight
}

// CalcWeight242 converts a display weight into the 0-242 client bar scale.
// Java L1PcInventory.calcWeight242(weight) lines 73-90: 0 when RATE_WEIGHT_LIMIT is 0;
// 242 when weight > maxWeight; otherwise Math.round(weight * 100 / maxWeight * 242 / 100).
// Stage thresholds used by Eddy's server:
//   - >= 121: HP regeneration stops (HpRegeneration.isOverWeight line 208)
//   - >= 120: MP regeneration stops (MpRegeneration.isOverWeight line 125)
//   - >= 197: attacking is refused (C_Attack line 39, S_ServerMessage 110)
func CalcWeight242(weight int64, maxWeight float64) byte {
	if maxWeight <= 0 || weight <= 0 {
		return 0
	}
	if float64(weight) >= maxWeight {
		return WeightBarMax
	}
	v := int(math.Round(float64(weight) * WeightBarMax / float64(maxWeight)))
	if v > WeightBarMax {
		v = WeightBarMax
	}
	return byte(v)
}

// Weight242 returns the current carried weight on the 0-242 client bar scale.
// Java: L1PcInventory.getWeight242() = calcWeight242(getWeight()).
func (inv *Inventory) Weight242(maxWeight float64) byte {
	return CalcWeight242(int64(inv.TotalWeight()), maxWeight)
}

// IsOverWeight reports whether adding `addWeight` display units (see AddWeight)
// would push the inventory to the full bar.
// Java L1PcInventory.checkAddItem() lines 110-122:
//
//	weight = getWeight() + templateWeight*count/1000 + 1
//	weight < 0 || templateWeight*count/1000 < 0 → WEIGHT_OVER
//	calcWeight242(weight) >= 242               → WEIGHT_OVER
func (inv *Inventory) IsOverWeight(addWeight int64, maxWeight float64) bool {
	if addWeight < 0 {
		return true
	}
	return CalcWeight242(int64(inv.TotalWeight())+addWeight+1, maxWeight) >= WeightBarMax
}

// IsOverWeightAdd is the convenience form of IsOverWeight for acquiring
// `count` units of a template whose weight is `unitWeight`.
func (inv *Inventory) IsOverWeightAdd(unitWeight, count int32, maxWeight float64) bool {
	return inv.IsOverWeight(AddWeight(unitWeight, count), maxWeight)
}

// ItemDescID returns the descId value for S_AddItem / S_AddInventoryBatch packets.
// Java S_AddItem.java: hardcoded switch(itemId) for specific material items.
// The 3.80C client uses this field for local spell material validation —
// without it, the client blocks casting (e.g. summon spell) before sending the packet.
func ItemDescID(itemID int32) uint16 {
	switch itemID {
	case 40318: // 魔法寶石 (Magic Gem) — summon spell material
		return 166
	case 40319: // 黑色血痕
		return 569
	case 40321: // 創造怪物的卷軸
		return 837
	case 49158:
		return 3674
	case 49157:
		return 3605
	case 49156:
		return 3606
	default:
		return 0
	}
}

// EffectiveBless returns the bless byte for inventory packets.
// Unidentified items are displayed as bless=3 (dark/gray name) by the client.
func EffectiveBless(item *InvItem) byte {
	if !item.Identified {
		if item.Bless >= 128 {
			return item.Bless // preserve sealed state
		}
		return 3 // unidentified
	}
	return item.Bless
}
