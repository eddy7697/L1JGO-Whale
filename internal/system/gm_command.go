package system

import (
	"strings"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// GMCommandSystem 處理 GM 命令的角色狀態修改。
// 實作 handler.GMCommandManager 介面。
type GMCommandSystem struct {
	deps *handler.Deps
}

// NewGMCommandSystem 建立 GM 命令系統。
func NewGMCommandSystem(deps *handler.Deps) *GMCommandSystem {
	return &GMCommandSystem{deps: deps}
}

// SetLevel 設定玩家等級（含經驗值、HP/MP 重算）。
func (s *GMCommandSystem) SetLevel(sess *net.Session, player *world.PlayerInfo, level int) {
	player.Dirty = true
	player.Level = int16(level)
	player.Exp = int32(s.deps.Scripting.ExpForLevel(level))

	// 重算基礎血魔時只用原始體質、精神；裝備及 buff 的加成仍有效，
	// 必須重新加回，否則脫裝或 buff 到期時會再次扣除而傷及基礎值。
	eq := player.EquipBonuses
	con, wis := player.Con-int16(eq.AddCon), player.Wis-int16(eq.AddWis)
	bonusHP, bonusMP := int32(eq.AddHP), int32(eq.AddMP)
	for _, buff := range player.ActiveBuffs {
		con -= buff.DeltaCon
		wis -= buff.DeltaWis
		bonusHP += buff.DeltaMaxHP
		bonusMP += buff.DeltaMaxMP
	}
	baseHP, baseMP := calcBaseHPMP(player.ClassType, player.Level, con, wis, s.deps)
	player.MaxHP = baseHP + bonusHP
	player.MaxMP = baseMP + bonusMP
	player.HP = player.MaxHP
	player.MP = player.MaxMP

	handler.SendPlayerStatus(sess, player)
	handler.SendExpUpdate(sess, player.Level, player.Exp)
	handler.SendHpUpdate(sess, player)
	handler.SendMpUpdate(sess, player)
}

// SetHP 設定玩家 HP（含死亡復活處理）。
func (s *GMCommandSystem) SetHP(sess *net.Session, player *world.PlayerInfo, hp int) {
	player.Dirty = true
	player.HP = int32(hp)
	if player.HP > player.MaxHP {
		player.MaxHP = player.HP
	}
	if player.HP > 0 && player.Dead {
		s.reviveGMPlayer(player)
	}
	handler.SendHpUpdate(sess, player)
	handler.SendPlayerStatus(sess, player)
}

// SetMP 設定玩家 MP。
func (s *GMCommandSystem) SetMP(sess *net.Session, player *world.PlayerInfo, mp int) {
	player.Dirty = true
	player.MP = int32(mp)
	if player.MP > player.MaxMP {
		player.MaxMP = player.MP
	}
	handler.SendMpUpdate(sess, player)
	handler.SendPlayerStatus(sess, player)
}

// FullHeal 補滿 HP/MP（含死亡復活處理）。
func (s *GMCommandSystem) FullHeal(sess *net.Session, player *world.PlayerInfo) {
	player.Dirty = true
	player.HP = player.MaxHP
	player.MP = player.MaxMP
	if player.Dead {
		s.reviveGMPlayer(player)
	}
	handler.SendHpUpdate(sess, player)
	handler.SendMpUpdate(sess, player)
}

func (s *GMCommandSystem) reviveGMPlayer(p *world.PlayerInfo) {
	p.Dead = false
	p.LastMoveTime = 0
	if s.deps.Death != nil {
		s.deps.Death.ClearPlayerTomb(p)
	}
	s.deps.World.OccupyEntity(p.MapID, p.X, p.Y, p.CharID)
	data := handler.BuildResurrection(p, p.CharID, 0)
	p.Session.Send(data)
	for _, v := range s.deps.World.GetNearbyPlayersInShow(p.X, p.Y, p.MapID, p.SessionID, p.ShowID) {
		if v.CanSeePlayer(p) {
			v.Session.Send(data)
			handler.SendPutObject(v.Session, p)
		}
	}
}

// SetStat 設定指定屬性值。
func (s *GMCommandSystem) SetStat(sess *net.Session, player *world.PlayerInfo, stat string, value int16) {
	player.Dirty = true
	switch strings.ToLower(stat) {
	case "str":
		player.Str = value
	case "dex":
		player.Dex = value
	case "con":
		player.Con = value
	case "wis":
		player.Wis = value
	case "int":
		player.Intel = value
	case "cha":
		player.Cha = value
	}
	handler.SendPlayerStatus(sess, player)
}

// GiveItem 給予物品。
func (s *GMCommandSystem) GiveItem(sess *net.Session, player *world.PlayerInfo, itemID, count int32, enchant int8) {
	itemInfo := s.deps.Items.Get(itemID)
	if itemInfo == nil {
		return
	}
	if count <= 0 {
		return
	}

	stackable := itemInfo.Stackable || itemID == world.AdenaItemID
	if stackable {
		if existing := player.Inv.FindByItemID(itemID); existing != nil && int64(existing.Count)+int64(count) > 2147483647 {
			return
		}
	}
	if s.deps.ItemCreate != nil {
		if stackable {
			if _, ok := s.deps.ItemCreate.GiveItem(sess, player, itemID, count); !ok {
				return
			}
			player.Dirty = true
			return
		}
		if creator, ok := s.deps.ItemCreate.(interface {
			GiveItemWithOptions(sess *net.Session, player *world.PlayerInfo, itemID, count int32, opts ItemCreateOptions) (*world.InvItem, bool)
		}); ok {
			opts := ItemCreateOptions{}
			if enchant != 0 {
				opts.EnchantLvl = enchant
			}
			for i := int32(0); i < count; i++ {
				if _, ok := creator.GiveItemWithOptions(sess, player, itemID, 1, opts); !ok {
					break
				}
				player.Dirty = true
			}
			return
		}
	}

	if stackable {
		existing := player.Inv.FindByItemID(itemID)
		wasExisting := existing != nil
		invItem := player.Inv.AddItem(
			itemID, count, itemInfo.Name, itemInfo.InvGfx,
			itemInfo.Weight, true, byte(itemInfo.Bless),
		)
		invItem.EnchantLvl = 0
		invItem.UseType = itemInfo.UseTypeID
		if itemInfo.MaxChargeCount > 0 {
			invItem.ChargeCount = int16(itemInfo.MaxChargeCount)
		}
		if wasExisting {
			handler.SendItemCountUpdate(sess, invItem)
		} else {
			handler.SendAddItem(sess, invItem)
		}
	} else {
		for i := int32(0); i < count; i++ {
			if player.Inv.IsFull() {
				break
			}
			invItem := player.Inv.AddItem(
				itemID, 1, itemInfo.Name, itemInfo.InvGfx,
				itemInfo.Weight, false, byte(itemInfo.Bless),
			)
			invItem.EnchantLvl = enchant
			invItem.UseType = itemInfo.UseTypeID
			if itemInfo.MaxChargeCount > 0 {
				invItem.ChargeCount = int16(itemInfo.MaxChargeCount)
			}
			handler.SendAddItem(sess, invItem)
		}
	}
	handler.SendWeightUpdate(sess, player)
	player.Dirty = true
}

// ApplyPoison GM 強制施加中毒。委派給 GMApplyPoison（system/poison.go）。
// ptype: 1=傷害毒, 2=沉默毒（卡司特毒）, 3=麻痺毒延遲。
func (s *GMCommandSystem) ApplyPoison(player *world.PlayerInfo, ptype byte) bool {
	return GMApplyPoison(player, ptype, s.deps)
}

// BreakWeapon GM 強制將玩家當前裝備武器的耐久損壞值設為 amount。
// amount 會夾在 1-127；無裝備武器則回傳 ("", false)。
// 成功時回傳武器名稱、true，並發送物品狀態更新封包讓客戶端重繪損壞顯示。
func (s *GMCommandSystem) BreakWeapon(player *world.PlayerInfo, amount int8) (string, bool) {
	if player == nil {
		return "", false
	}
	weapon := player.Equip.Weapon()
	if weapon == nil {
		return "", false
	}
	if amount < 1 {
		amount = 1
	}
	if amount > 127 {
		amount = 127
	}
	weapon.Durability = amount
	handler.SendItemCountUpdate(player.Session, weapon)
	player.Dirty = true
	return weapon.Name, true
}

// GiveGold 給予金幣。
func (s *GMCommandSystem) GiveGold(sess *net.Session, player *world.PlayerInfo, amount int32) {
	adenaInfo := s.deps.Items.Get(world.AdenaItemID)
	if adenaInfo == nil {
		return
	}
	if s.deps.ItemCreate != nil {
		s.deps.ItemCreate.GiveItem(sess, player, world.AdenaItemID, amount)
		return
	}

	existing := player.Inv.FindByItemID(world.AdenaItemID)
	wasExisting := existing != nil

	invItem := player.Inv.AddItem(
		world.AdenaItemID, amount, adenaInfo.Name, adenaInfo.InvGfx,
		0, true, byte(adenaInfo.Bless),
	)

	if wasExisting {
		handler.SendItemCountUpdate(sess, invItem)
	} else {
		handler.SendAddItem(sess, invItem)
	}
	handler.SendWeightUpdate(sess, player)
}

// AdjustLawful 以 signed delta 調整玩家正義值，並送出 Java S_Lawful 相同格式。
func (s *GMCommandSystem) AdjustLawful(sess *net.Session, player *world.PlayerInfo, delta int32) {
	if player == nil {
		return
	}
	next := int64(player.Lawful) + int64(delta)
	switch {
	case next > 32767:
		player.Lawful = 32767
	case next < -32768:
		player.Lawful = -32768
	default:
		player.Lawful = int32(next)
	}
	player.Dirty = true

	data := handler.BuildLawful(player.CharID, player.Lawful)
	if s.deps != nil && s.deps.World != nil {
		nearby := s.deps.World.GetNearbyPlayersAt(player.X, player.Y, player.MapID)
		if len(nearby) > 0 {
			handler.BroadcastToPlayers(nearby, data)
			return
		}
	}
	if sess != nil {
		sess.Send(data)
	}
}

// calcBaseHPMP 計算指定等級的基礎 HP/MP（透過 Lua 重複升級計算）。
func calcBaseHPMP(classType int16, level int16, con, wis int16, deps *handler.Deps) (int32, int32) {
	hp := int32(deps.Scripting.CalcInitHP(int(classType), int(con)))
	mp := int32(deps.Scripting.CalcInitMP(int(classType), int(wis)))

	for lv := int16(2); lv <= level; lv++ {
		result := deps.Scripting.CalcLevelUp(int(classType), int(con), int(wis))
		hp += int32(result.HP)
		mp += int32(result.MP)
	}
	return hp, mp
}
