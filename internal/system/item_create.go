package system

import (
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// ItemCreateSystem 提供統一給玩家物品入口。
type ItemCreateSystem struct {
	deps *handler.Deps
}

// ItemCreateOptions 描述給物品時需要覆寫的實例欄位。
type ItemCreateOptions struct {
	EnchantLvl int8
	Bless      byte
	BlessSet   bool
	SingleItem bool
	BeforeSend func(*world.InvItem)
}

// NewItemCreateSystem 建立 ItemCreateSystem。
func NewItemCreateSystem(deps *handler.Deps) *ItemCreateSystem {
	return &ItemCreateSystem{deps: deps}
}

// GiveItem 給玩家物品，處理 Java L1Inventory.storeItem 會負責的基本背包語義。
func (s *ItemCreateSystem) GiveItem(sess *net.Session, player *world.PlayerInfo, itemID, count int32) (*world.InvItem, bool) {
	return s.GiveItemWithOptions(sess, player, itemID, count, ItemCreateOptions{})
}

// GiveItemWithOptions 給玩家物品，並在送出背包封包前套用實例欄位覆寫。
func (s *ItemCreateSystem) GiveItemWithOptions(sess *net.Session, player *world.PlayerInfo, itemID, count int32, opts ItemCreateOptions) (*world.InvItem, bool) {
	if s == nil || s.deps == nil || s.deps.Items == nil || player == nil || player.Inv == nil {
		return nil, false
	}
	if count <= 0 {
		return nil, false
	}

	info := s.deps.Items.Get(itemID)
	if info == nil {
		return nil, false
	}

	stackable := info.Stackable || itemID == world.AdenaItemID
	newSlots := int32(1)
	if opts.SingleItem {
		newSlots = 1
	} else if stackable && player.Inv.FindByItemID(itemID) != nil {
		newSlots = 0
	} else if !stackable {
		newSlots = count
	}
	if newSlots > 0 && player.Inv.Size()+int(newSlots) > world.MaxInventorySize {
		if sess != nil {
			handler.SendServerMessage(sess, 263)
		}
		return nil, false
	}

	if info.Weight > 0 && player.Inv.IsOverWeightAdd(info.Weight, count, world.PlayerMaxWeight(player)) {
		if sess != nil {
			handler.SendServerMessage(sess, 82)
		}
		return nil, false
	}

	var last *world.InvItem
	if opts.SingleItem {
		last = player.Inv.AddItem(itemID, count, info.Name, info.InvGfx, info.Weight, false, byte(info.Bless))
		applyItemTemplate(last, info)
		applyItemCreateOptions(last, opts)
		if opts.BeforeSend != nil {
			opts.BeforeSend(last)
		}
		if sess != nil {
			handler.SendAddItem(sess, last, info)
		}
	} else if stackable {
		existing := player.Inv.FindByItemID(itemID)
		wasExisting := existing != nil
		last = player.Inv.AddItem(itemID, count, info.Name, info.InvGfx, info.Weight, true, byte(info.Bless))
		applyItemTemplate(last, info)
		applyItemCreateOptions(last, opts)
		if opts.BeforeSend != nil {
			opts.BeforeSend(last)
		}
		if sess != nil {
			if wasExisting {
				handler.SendItemCountUpdate(sess, last)
			} else {
				handler.SendAddItem(sess, last, info)
			}
		}
	} else {
		for i := int32(0); i < count; i++ {
			last = player.Inv.AddItem(itemID, 1, info.Name, info.InvGfx, info.Weight, false, byte(info.Bless))
			applyItemTemplate(last, info)
			applyItemCreateOptions(last, opts)
			if opts.BeforeSend != nil {
				opts.BeforeSend(last)
			}
			if sess != nil {
				handler.SendAddItem(sess, last, info)
			}
		}
	}

	if sess != nil {
		handler.SendWeightUpdate(sess, player)
	}
	return last, true
}

func applyItemTemplate(item *world.InvItem, info *data.ItemInfo) {
	if item == nil || info == nil {
		return
	}
	item.UseType = info.UseTypeID
	if info.MaxChargeCount > 0 && item.ChargeCount == 0 {
		item.ChargeCount = int16(info.MaxChargeCount)
	}
}

func applyItemCreateOptions(item *world.InvItem, opts ItemCreateOptions) {
	if item == nil {
		return
	}
	if opts.EnchantLvl != 0 {
		item.EnchantLvl = opts.EnchantLvl
	}
	if opts.BlessSet {
		item.Bless = opts.Bless
	}
}
