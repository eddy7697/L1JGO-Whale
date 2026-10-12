package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// dispatchGMWorld 對照 Java command/executor 的世界管理與診斷指令。
// 權限檢查由 HandleGMCommand 的共同入口處理，包括 .f 展開後的第二次檢查。
func dispatchGMWorld(sess *net.Session, player *world.PlayerInfo, cmd string, args []string, deps *Deps) bool {
	switch cmd {
	case "gm":
		enabled := !player.IsGM()
		if len(args) > 0 {
			if len(args) != 1 || (!strings.EqualFold(args[0], "on") && !strings.EqualFold(args[0], "off")) {
				gmMsg(sess, "用法: .gm [on|off]")
				break
			}
			enabled = strings.EqualFold(args[0], "on")
		}
		player.SetGM(enabled)
		gmMsgf(sess, "setGm = %t", player.IsGM())
	case "hpbar", "showtrap", "findinvis":
		gmWorldToggle(sess, player, cmd, args, deps)
	case "resettrap":
		if deps.TrapMgr == nil {
			gmMsg(sess, "陷阱系統未初始化")
			break
		}
		deps.TrapMgr.ResetAllTraps()
		deps.World.AllPlayers(func(p *world.PlayerInfo) { UpdateGMWorldView(p, deps) })
		gmMsg(sess, "陷阱已被重新分配")
	case "reloadtrap":
		if deps.TrapMgr == nil {
			gmMsg(sess, "陷阱系統未初始化")
			break
		}
		traps, err := data.LoadTrapData("data/yaml")
		if err == nil {
			err = traps.ValidateGMReload()
		}
		if err != nil {
			gmMsgf(sess, "重新讀取陷阱失敗: %v", err)
			break
		}
		deps.TrapMgr.Reload(traps)
		deps.World.AllPlayers(func(p *world.PlayerInfo) { UpdateGMWorldView(p, deps) })
		gmMsg(sess, "已重新讀取陷阱資料")
	case "castgfx":
		id, ok := gmWorldNumber(args, 0, 0, 65535)
		if !ok {
			gmMsg(sess, "用法: .castgfx <動畫ID 0-65535>")
			break
		}
		SendSkillEffect(sess, player.CharID, int32(id))
		payload := BuildSkillEffect(player.CharID, int32(id))
		for _, viewer := range deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, player.SessionID, player.ShowID) {
			if viewer.CanSeePlayer(player) {
				viewer.Session.Send(payload)
			}
		}
	case "gfxid":
		gmWorldGfx(sess, player, args, deps, false)
	case "invgfxid":
		gmWorldGfx(sess, player, args, deps, true)
	case "patrol":
		gmPatrol(sess, deps)
	case "gmroom":
		gmRoom(sess, player, args, deps)
	case "tospawn":
		gmToSpawn(sess, player, args, deps)
	case "f":
		gmFavorite(sess, player, args, deps)
	case "partyrecall":
		gmPartyRecall(sess, player, args, deps)
	case "insert":
		gmInsertSpawn(sess, player, args, deps)
	case "action":
		id, ok := gmWorldNumber(args, 0, 0, 255)
		if !ok {
			gmMsg(sess, "用法: .action <動作ID 0-255>")
			break
		}
		SendActionGfx(sess, player.CharID, byte(id))
	case "tile":
		if deps.MapData == nil {
			gmMsg(sess, "地圖資料未載入")
			break
		}
		dirs := [8][2]int32{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}
		parts := make([]string, 0, len(dirs))
		for i, d := range dirs {
			parts = append(parts, fmt.Sprintf("%d:%d", i, deps.MapData.OriginalTile(player.MapID, player.X+d[0], player.Y+d[1])))
		}
		gmMsg(sess, strings.Join(parts, " "))
	case "itemset":
		gmItemSet(sess, player, args, deps)
	case "summon":
		gmSummon(sess, player, args, deps)
	default:
		return false
	}
	return true
}

func gmWorldNumber(args []string, index, min, max int) (int, bool) {
	if index >= len(args) {
		return 0, false
	}
	v, err := strconv.Atoi(args[index])
	return v, err == nil && v >= min && v <= max
}

func gmWorldToggle(sess *net.Session, p *world.PlayerInfo, cmd string, args []string, deps *Deps) {
	if len(args) != 1 || (!strings.EqualFold(args[0], "on") && !strings.EqualFold(args[0], "off")) {
		gmMsgf(sess, "用法: .%s on|off", cmd)
		return
	}
	on := strings.EqualFold(args[0], "on")
	switch cmd {
	case "hpbar":
		p.GMHPBar = on
	case "showtrap":
		p.GMShowTraps = on
	case "findinvis":
		p.GMFindInvis = on
		for _, other := range deps.World.GetNearbyPlayersInShow(p.X, p.Y, p.MapID, p.SessionID, p.ShowID) {
			if !other.IsInvisible() {
				continue
			}
			if p.CanSeePlayer(other) {
				SendPutObject(sess, other)
				if p.Known != nil {
					p.Known.Players[other.CharID] = world.KnownPos{X: other.X, Y: other.Y}
				}
			} else {
				SendRemoveObject(sess, other.CharID)
				if p.Known != nil {
					delete(p.Known.Players, other.CharID)
				}
			}
		}
	}
	UpdateGMWorldView(p, deps)
	gmMsgf(sess, "%s = %t", cmd, on)
}

func gmWorldGfx(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps, inventory bool) {
	id, ok := gmWorldNumber(args, 0, 0, 65535)
	count, okCount := gmWorldNumber(args, 1, 1, 100)
	if !ok || !okCount || id+count-1 > 65535 {
		gmMsg(sess, "用法: .gfxid 或 .invgfxid <起始ID 0-65535> <數量1-100>")
		return
	}
	if !inventory {
		manager, ok := deps.GMCmd.(interface {
			SpawnGMDebugGfx(*world.PlayerInfo, int32, int) error
		})
		if !ok {
			gmMsg(sess, "GM 圖形測試系統未初始化")
			return
		}
		if err := manager.SpawnGMDebugGfx(p, int32(id), count); err != nil {
			gmMsgf(sess, "圖形建立失敗: %v", err)
		}
		return
	}
	if deps.Items == nil || p.Inv == nil {
		gmMsg(sess, "物品資料未載入")
		return
	}
	tmpl := deps.Items.Get(40005)
	if tmpl == nil {
		gmMsg(sess, "找不到測試物品模板 40005")
		return
	}
	if len(p.Inv.Items)+count > world.MaxInventorySize {
		gmMsg(sess, "背包空間不足")
		return
	}
	for i := 0; i < count; i++ {
		item := p.Inv.AddItem(40005, 1, strconv.Itoa(id+i), int32(id+i), tmpl.Weight, false, byte(tmpl.Bless))
		item.UseType = tmpl.UseTypeID
		SendAddItem(sess, item)
	}
	p.Dirty = true
	SendWeightUpdate(sess, p)
}

func gmPatrol(sess *net.Session, deps *Deps) {
	var players []*world.PlayerInfo
	deps.World.AllPlayers(func(p *world.PlayerInfo) { players = append(players, p) })
	sort.Slice(players, func(i, j int) bool { return players[i].Name < players[j].Name })
	if len(players) > 255 {
		gmMsg(sess, "巡查視窗最多顯示255位，請配合 .who 查詢")
		players = players[:255]
	}
	var lastActive map[string]time.Time
	if deps.AccountRepo != nil {
		names := make([]string, 0, len(players))
		for _, p := range players {
			if p.Session != nil {
				names = append(names, p.Session.AccountName)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		var err error
		lastActive, err = deps.AccountRepo.LastActiveByNames(ctx, names)
		cancel()
		if err != nil {
			gmMsgf(sess, "巡查登入時間查詢失敗: %v", err)
			return
		}
	}
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_EVENT)
	w.WriteC(45) // Java S_PacketBox.CALL_SOMETHING
	w.WriteC(byte(len(players)))
	for _, p := range players {
		active := time.Time{}
		if p.Session != nil {
			active = lastActive[p.Session.AccountName]
		}
		w.WriteD(gmPatrolTimestamp(active))
		w.WriteS(p.Name)
		w.WriteS(p.ClanName)
	}
	sess.Send(w.Bytes())
}

func gmPatrolTimestamp(active time.Time) int32 {
	if active.IsZero() {
		return 0
	}
	// Java Config.TIME_ZONE=Asia/Taipei，Calendar.YEAR=1970，保留月日時分秒。
	local := active.In(time.FixedZone("Asia/Taipei", 8*60*60))
	return int32(time.Date(1970, local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), 0, local.Location()).Unix())
}

func gmRoom(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) != 1 {
		gmMsg(sess, "用法: .gmroom <1-5|地點名稱>")
		return
	}
	rooms := map[string]data.GMRoom{"1": {X: 32737, Y: 32796, MapID: 99}, "2": {X: 32734, Y: 32799, MapID: 17100}, "3": {X: 32644, Y: 32955, MapID: 0}, "4": {X: 33429, Y: 32814, MapID: 4}, "5": {X: 32894, Y: 32535, MapID: 300}}
	r, ok := rooms[args[0]]
	if !ok && deps.GMConfig != nil {
		r, ok = deps.GMConfig.Rooms[strings.ToLower(args[0])]
	}
	if !ok {
		gmMsgf(sess, "%s 未定義的 Room", args[0])
		return
	}
	teleportPlayerInShow(sess, p, r.X, r.Y, r.MapID, 5, 0, deps)
}

func gmToSpawn(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps) {
	if deps.GMSpawns == nil {
		gmMsg(sess, "生成點資料未初始化")
		return
	}
	id := int64(p.GMSpawnID)
	if len(args) == 0 || args[0] == "+" {
		id++
	} else if args[0] == "-" {
		id--
	} else {
		parsed, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil {
			gmMsg(sess, "用法: .tospawn <生成點ID|+|->")
			return
		}
		id = parsed
	}
	if id < 0 || id > 2147483647 {
		gmMsg(sess, "生成點ID超出範圍")
		return
	}
	p.GMSpawnID = int32(id)
	entry, ok, legacy := deps.GMSpawns.Lookup(int32(id))
	if !ok {
		gmMsgf(sess, "spawnid(%d)找不到", id)
		return
	}
	teleportPlayerInShow(sess, p, entry.X, entry.Y, entry.MapID, 5, 0, deps)
	if legacy {
		gmMsgf(sess, "已傳送到生成點 %d（舊 YAML 項次，未保留 Java 原始 ID）", id)
	} else {
		gmMsgf(sess, "spawnid(%d)已傳送到", id)
	}
}

func gmFavorite(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) > 0 && strings.EqualFold(args[0], "set") {
		if len(args) < 2 {
			gmMsg(sess, "用法: .f set <指令> [參數；%為代入位置]")
			return
		}
		cmd := strings.ToLower(strings.TrimPrefix(args[1], "."))
		spec := gmCommandIndex[cmd]
		if spec == nil {
			gmMsg(sess, "指令不存在，無法登記最愛")
			return
		}
		if spec.Name == "f" || spec.Name == "r" {
			gmMsg(sess, "不能將最愛或重複指令登記為最愛")
			return
		}
		parts := append([]string{spec.Name}, args[2:]...)
		p.GMFavorite = strings.Join(parts, " ")
		gmMsgf(sess, "已登記指令: %s", p.GMFavorite)
		return
	}
	if len(args) > 0 && strings.EqualFold(args[0], "show") {
		gmMsgf(sess, "目前登記的指令: %s", p.GMFavorite)
		return
	}
	if p.GMFavorite == "" {
		gmMsg(sess, "沒有登記的指令")
		return
	}
	parts := strings.Fields(p.GMFavorite)
	used := 0
	for i, part := range parts {
		if strings.HasPrefix(part, "%") {
			if used >= len(args) {
				gmMsg(sess, "最愛指令的代入參數不足")
				return
			}
			parts[i] = args[used]
			used++
		}
	}
	if len(parts) == 0 {
		gmMsg(sess, "沒有登記的指令")
		return
	}
	spec := gmCommandIndex[strings.ToLower(strings.TrimPrefix(parts[0], "."))]
	if spec == nil || spec.Name == "f" || spec.Name == "r" {
		gmMsg(sess, "不能遞迴執行最愛指令")
		return
	}
	parts = append(parts, args[used:]...)
	HandleGMCommand(sess, p, "."+strings.Join(parts, " "), deps)
}

func gmPartyRecall(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) != 1 {
		gmMsg(sess, "用法: .partyrecall <隊員角色名稱>")
		return
	}
	target := deps.World.GetByName(args[0])
	if target == nil {
		gmMsg(sess, "角色不在線上")
		return
	}
	party := deps.World.Parties.GetParty(target.CharID)
	if party == nil {
		gmMsg(sess, "指定角色沒有隊伍")
		return
	}
	x, y, mapID, showID := p.X, p.Y+2, p.MapID, p.ShowID
	members := append([]int32(nil), party.Members...)
	for _, id := range members {
		if member := deps.World.GetByCharID(id); member != nil {
			teleportPlayerInShow(member.Session, member, x, y, mapID, 5, showID, deps)
			gmMsg(member.Session, "您被傳喚到 GM 身邊。")
		}
	}
}

func gmInsertSpawn(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps) {
	npcID, ok := gmWorldNumber(args, 1, 1, 2147483647)
	if len(args) != 2 || !ok || (strings.ToLower(args[0]) != "mob" && strings.ToLower(args[0]) != "npc") {
		gmMsg(sess, "用法: .insert mob|npc <NPCID>")
		return
	}
	manager, ok := deps.GMCmd.(interface {
		InsertGMSpawn(*world.PlayerInfo, string, int32) (int32, error)
	})
	if !ok {
		gmMsg(sess, "GM 生成點系統未初始化")
		return
	}
	id, err := manager.InsertGMSpawn(p, strings.ToLower(args[0]), int32(npcID))
	if err != nil {
		gmMsgf(sess, "新增生成點失敗: %v", err)
		return
	}
	gmMsgf(sess, "NPC %d 已永久新增，生成點 ID=%d", npcID, id)
}

func gmItemSet(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) != 1 {
		gmMsg(sess, "用法: .itemset <套裝名稱>")
		return
	}
	if deps.GMConfig == nil || deps.Items == nil || deps.GMCmd == nil || p.Inv == nil {
		gmMsg(sess, "GM 套裝資料未初始化")
		return
	}
	items, ok := deps.GMConfig.ItemSets[args[0]]
	if !ok {
		gmMsgf(sess, "%s 是未定義的套裝", args[0])
		return
	}
	slots := int64(len(p.Inv.Items))
	for _, i := range items {
		tmpl := deps.Items.Get(i.ID)
		if tmpl == nil {
			gmMsgf(sess, "套裝含不存在的物品 %d", i.ID)
			return
		}
		if tmpl.Stackable || i.ID == world.AdenaItemID {
			if p.Inv.FindByItemID(i.ID) == nil {
				slots++
			}
		} else {
			slots += int64(i.Amount)
		}
	}
	if slots > world.MaxInventorySize {
		gmMsg(sess, "背包空間不足，未發放套裝")
		return
	}
	for _, i := range items {
		deps.GMCmd.GiveItem(sess, p, i.ID, i.Amount, i.Enchant)
	}
}

func gmSummon(sess *net.Session, p *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) == 0 || len(args) > 2 {
		gmMsg(sess, "用法: .summon <NPCID|名稱> [數量1-100]")
		return
	}
	if deps.Npcs == nil {
		gmMsg(sess, "NPC 資料未載入")
		return
	}
	var tmpl *data.NpcTemplate
	if id, err := strconv.ParseInt(args[0], 10, 32); err == nil {
		tmpl = deps.Npcs.Get(int32(id))
	} else {
		tmpl = deps.Npcs.FindByNameWithoutSpace(args[0])
	}
	if tmpl == nil {
		gmMsg(sess, "找不到符合條件的 NPC")
		return
	}
	count := 1
	if len(args) > 1 {
		var ok bool
		count, ok = gmWorldNumber(args, 1, 1, 100)
		if !ok {
			gmMsg(sess, "召喚數量必須為1-100")
			return
		}
	}
	manager, ok := deps.Summon.(interface {
		GMSummon(*world.PlayerInfo, int32, int) error
	})
	if !ok {
		gmMsg(sess, "GM 召喚系統未初始化")
		return
	}
	if err := manager.GMSummon(p, tmpl.NpcID, count); err != nil {
		gmMsgf(sess, "召喚失敗: %v", err)
		return
	}
	gmMsgf(sess, "%s(ID:%d) (%d) 召喚了", tmpl.Name, tmpl.NpcID, count)
}

// UpdateGMWorldView 由既有 VisibilitySystem 單執行緒更新，避免額外計時 goroutine。
func UpdateGMWorldView(p *world.PlayerInfo, deps *Deps) {
	if p == nil || p.Session == nil || p.Known == nil || deps == nil || deps.World == nil {
		return
	}
	updateGMHPBars(p, deps)
	updateGMTraps(p, deps)
}

func updateGMHPBars(p *world.PlayerInfo, deps *Deps) {
	if !p.GMHPBar && len(p.Known.GMHPBars) == 0 {
		return
	}
	if p.Known.GMHPBars == nil {
		p.Known.GMHPBars = make(map[int32]int16)
	}
	current := make(map[int32]int16)
	put := func(id, hp, maxHP int32) {
		ratio := int16(0)
		if maxHP > 0 {
			n := int64(hp) * 100 / int64(maxHP)
			if n < 0 {
				n = 0
			}
			if n > 100 {
				n = 100
			}
			ratio = int16(n)
		}
		current[id] = ratio
		if prev, ok := p.Known.GMHPBars[id]; !ok || prev != ratio {
			SendHpMeter(p.Session, id, ratio)
		}
	}
	if p.GMHPBar {
		for _, v := range deps.World.GetNearbyPlayersInShow(p.X, p.Y, p.MapID, p.SessionID, p.ShowID) {
			if p.CanSeePlayer(v) {
				put(v.CharID, v.HP, v.MaxHP)
			}
		}
		for _, v := range deps.World.GetNearbyNpcsForVisInShow(p.X, p.Y, p.MapID, p.ShowID) {
			if v.Impl == "L1Monster" {
				put(v.ID, v.HP, v.MaxHP)
			}
		}
		for _, v := range deps.World.GetNearbySummonsInShow(p.X, p.Y, p.MapID, p.ShowID) {
			put(v.ID, v.HP, v.MaxHP)
		}
		for _, v := range deps.World.GetNearbyPetsInShow(p.X, p.Y, p.MapID, p.ShowID) {
			put(v.ID, v.HP, v.MaxHP)
		}
	}
	for id := range p.Known.GMHPBars {
		if _, ok := current[id]; !ok {
			SendHpMeter(p.Session, id, 255)
		}
	}
	p.Known.GMHPBars = current
}

func updateGMTraps(p *world.PlayerInfo, deps *Deps) {
	if !p.GMShowTraps && len(p.Known.GMTraps) == 0 {
		return
	}
	if p.Known.GMTraps == nil {
		p.Known.GMTraps = make(map[int32]world.KnownPos)
	}
	current := make(map[int32]world.KnownPos)
	if p.GMShowTraps && deps.TrapMgr != nil {
		for _, t := range deps.TrapMgr.GetVisibleTrapsInScreen(p.X, p.Y, p.MapID) {
			pos := world.KnownPos{X: t.X, Y: t.Y}
			current[t.ID] = pos
			if prev, ok := p.Known.GMTraps[t.ID]; !ok || prev != pos {
				if ok {
					SendRemoveObject(p.Session, t.ID)
				}
				sendGMTrap(p.Session, t)
			}
		}
	}
	for id := range p.Known.GMTraps {
		if _, ok := current[id]; !ok {
			SendRemoveObject(p.Session, id)
		}
	}
	p.Known.GMTraps = current
}

func sendGMTrap(sess *net.Session, t *world.TrapInstance) {
	// Java S_Trap 使用 S_OPCODE_DROPITEM，即本服的 S_OPCODE_PUT_OBJECT。
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_PUT_OBJECT)
	w.WriteH(uint16(t.X))
	w.WriteH(uint16(t.Y))
	w.WriteD(t.ID)
	w.WriteH(7)
	w.WriteC(0)
	w.WriteC(0)
	w.WriteC(0)
	w.WriteC(0)
	w.WriteD(0)
	w.WriteC(0)
	w.WriteC(0)
	name := "trap"
	if t.Base {
		name = "trap base"
	}
	w.WriteS(name)
	w.WriteC(0)
	w.WriteD(0)
	w.WriteD(0)
	w.WriteC(255)
	w.WriteC(0)
	w.WriteC(0)
	w.WriteC(0)
	w.WriteH(65535)
	w.WriteD(0)
	w.WriteC(8)
	w.WriteC(0)
	sess.Send(w.Bytes())
}
