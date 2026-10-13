package handler

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// HandleGMCommand processes a "." prefixed GM command.
// Returns true if the text was a GM command (consumed), false otherwise.
func HandleGMCommand(sess *net.Session, player *world.PlayerInfo, text string, deps *Deps) bool {
	if !strings.HasPrefix(text, ".") {
		return false
	}
	// Java C_Chat only routes commands for GM/Monitor. Check here as well so
	// another packet handler cannot accidentally bypass authorization.
	if sess == nil || player == nil || !player.CanUseGMCommands() {
		return false
	}

	// Parse command and arguments
	parts := strings.Fields(text[1:]) // strip leading "."
	if len(parts) == 0 {
		return true
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	spec := gmCommandIndex[cmd]
	if spec == nil {
		if cmd == "crazy" || cmd == "狂暴" {
			gmMsg(sess, "Java 指令表雖列出 crazy，但參考專案缺少 L1Crazy 實作，無可對照的行為。")
			return true
		}
		gmMsg(sess, "未知的 GM 指令：."+cmd+"，輸入 .help 查看指令。")
		return true
	}
	if player.AccessLevel < spec.Level {
		gmMsgf(sess, "你的權限不足以使用 .%s（需要 %d）。", spec.Name, spec.Level)
		return true
	}
	cmd = spec.Name
	if cmd == "r" {
		if player.LastGMCommand == "" {
			gmMsg(sess, "尚無可重複執行的指令。")
			return true
		}
		last := player.LastGMCommand
		if len(args) > 0 {
			last = strings.Fields(last)[0] + " " + strings.Join(args, " ")
		}
		// Re-enter authorization: a previously allowed command may no longer be allowed.
		return HandleGMCommand(sess, player, "."+last, deps)
	}
	player.LastGMCommand = strings.Join(append([]string{cmd}, args...), " ")
	if deps != nil && deps.Log != nil {
		deps.Log.Info("GM 指令", zap.Int32("char_id", player.CharID), zap.String("player", player.Name),
			zap.Int16("access_level", player.AccessLevel), zap.String("command", cmd), zap.Strings("args", args))
	}
	if dispatchGMAdmin(sess, player, cmd, args, deps) || dispatchGMWorld(sess, player, cmd, args, deps) || dispatchGMCharacter(sess, player, cmd, args, deps) || dispatchGMPoints(sess, player, cmd, args, deps) {
		return true
	}

	switch cmd {
	case "cpshopreload":
		gmCPShopReload(sess, deps)
	case "help":
		gmCommandHelp(sess, player, args)
	case "level":
		gmLevel(sess, player, args, deps)
	case "hp":
		gmHP(sess, player, args, deps)
	case "mp":
		gmMP(sess, player, args, deps)
	case "heal":
		gmHeal(sess, player, deps)
	case "stat":
		gmStat(sess, player, args, deps)
	case "move", "warp", "teleport":
		gmMove(sess, player, args, deps)
	case "item":
		gmItem(sess, player, args, deps)
	case "gold", "adena":
		gmGold(sess, player, args, deps)
	case "lawful", "law", "l":
		gmLawful(sess, player, args, deps)
	case "spell":
		gmSpell(sess, player, args, deps)
	case "addskill":
		gmAllSkill(sess, player, deps)
	case "spawn":
		gmSpawn(sess, player, args, deps)
	case "kill":
		gmKill(sess, player, args, deps)
	case "killall":
		gmKillAll(sess, player, deps)
	case "speed":
		gmSpeed(sess, player, args, deps)
	case "who":
		gmWho(sess, deps)
	case "topc":
		gmGoto(sess, player, args, deps)
	case "recall":
		gmRecall(sess, player, args, deps)
	case "exp":
		gmExp(sess, player, args, deps)
	case "class":
		gmClass(sess, player, args, deps)
	case "rez", "resurrect":
		gmRez(sess, player, args, deps)
	case "ac":
		gmShowInfo(sess, player)
	case "poly":
		gmPoly(sess, player, args, deps)
	case "polygfx":
		gmPolyGfx(sess, player, args, deps)
	case "undopoly":
		gmUndoPoly(sess, player, args, deps)
	case "loc", "pos", "coord":
		gmLoc(sess, player, args, deps)
	case "wall":
		gmWall(sess, player, args, deps)
	case "clearwall":
		gmClearWall(sess, player, deps)
	case "weather":
		gmWeather(sess, player, args, deps)
	case "poison":
		gmPoison(sess, player, args, deps)
	case "broken":
		gmBroken(sess, player, args, deps)
	case "water":
		gmWater(sess, player, args, deps)
	case "stresstest":
		gmStressTest(sess, player, args, deps)
	case "cleartest":
		gmClearTest(sess, player, deps)
	case "invisible":
		gmInvisible(sess, player, deps)
	case "visible":
		gmSetInvisible(sess, player, false, deps)
	case "slottest":
		gmSlotTest(sess, player, args)
	case "slotexpand":
		gmSlotExpand(sess, args)
	case "slotexpand2":
		gmSlotExpand2(sess, args)
	case "time":
		gmTime(sess, player, args, deps)
	default:
		gmMsg(sess, "\\f3未知的GM指令: ."+cmd+"  輸入 .help 查看指令列表")
	}

	return true
}

// --- Helper ---

func gmMsg(sess *net.Session, msg string) {
	sendGlobalChat(sess, 9, msg) // type 9 = system message (green text)
}

func gmMsgf(sess *net.Session, format string, a ...any) {
	gmMsg(sess, fmt.Sprintf(format, a...))
}

// --- Commands ---

func gmLevel(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .level <等級>")
		return
	}
	lv, err := strconv.Atoi(args[0])
	if err != nil || lv < 1 || lv > 99 {
		gmMsg(sess, "\\f3等級必須在 1-99 之間")
		return
	}

	deps.GMCmd.SetLevel(sess, player, lv)

	gmMsgf(sess, "等級已設為 %d (HP:%d MP:%d)", lv, player.MaxHP, player.MaxMP)
}

func gmHP(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .hp <數值>")
		return
	}
	val, err := strconv.Atoi(args[0])
	if err != nil || val < 0 || int64(val) > 2147483647 {
		gmMsg(sess, "\\f3無效的HP數值")
		return
	}

	deps.GMCmd.SetHP(sess, player, val)
	gmMsgf(sess, "HP 已設為 %d/%d", player.HP, player.MaxHP)
}

func gmMP(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .mp <數值>")
		return
	}
	val, err := strconv.Atoi(args[0])
	if err != nil || val < 0 || int64(val) > 2147483647 {
		gmMsg(sess, "\\f3無效的MP數值")
		return
	}

	deps.GMCmd.SetMP(sess, player, val)
	gmMsgf(sess, "MP 已設為 %d/%d", player.MP, player.MaxMP)
}

func gmHeal(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	deps.GMCmd.FullHeal(sess, player)
	gmMsg(sess, "HP/MP 已補滿")
}

func gmStat(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 2 {
		gmMsg(sess, "\\f3用法: .stat <str|dex|con|wis|int|cha> <數值>")
		return
	}
	val, err := strconv.Atoi(args[1])
	if err != nil || val < 1 || val > 127 {
		gmMsg(sess, "\\f3屬性數值必須在 1-127 之間")
		return
	}

	stat := strings.ToLower(args[0])
	v := int16(val)
	switch stat {
	case "str", "dex", "con", "wis", "int", "cha":
		deps.GMCmd.SetStat(sess, player, stat, v)
	default:
		gmMsg(sess, "\\f3未知的屬性: "+stat)
		return
	}

	gmMsgf(sess, "%s 已設為 %d", strings.ToUpper(stat), val)
}

func gmMove(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 2 {
		gmMsg(sess, "\\f3用法: .move <x> <y> [mapID]")
		return
	}
	x, err := strconv.Atoi(args[0])
	if err != nil {
		gmMsg(sess, "\\f3無效的X座標")
		return
	}
	y, err := strconv.Atoi(args[1])
	if err != nil {
		gmMsg(sess, "\\f3無效的Y座標")
		return
	}
	mapID := int(player.MapID)
	if len(args) >= 3 {
		mapID, err = strconv.Atoi(args[2])
		if err != nil {
			gmMsg(sess, "\\f3無效的地圖ID")
			return
		}
	}
	if x < 0 || x > 65535 || y < 0 || y > 65535 || mapID < 0 || mapID > 32767 {
		gmMsg(sess, "座標或地圖編號超出範圍。")
		return
	}
	if deps.MapData != nil {
		info := deps.MapData.GetInfo(int16(mapID))
		if info == nil || int32(x) < info.StartX || int32(x) > info.EndX || int32(y) < info.StartY || int32(y) > info.EndY {
			gmMsg(sess, "目的地不在已知地圖範圍內。")
			return
		}
	}

	teleportPlayer(sess, player, int32(x), int32(y), int16(mapID), 5, deps)
	gmMsgf(sess, "已傳送至 (%d, %d) 地圖 %d", x, y, mapID)
}

func gmItem(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .item <itemID|中文名> [數量] 或 .item <物品> +強化 [數量]")
		return
	}
	itemArg, count, enchant, enchantSpecified, parseErr := parseGMItemArgs(args)
	if parseErr != "" {
		gmMsg(sess, "\\f3"+parseErr)
		return
	}

	itemInfo := resolveItemArg(deps, itemArg)
	if itemInfo == nil {
		// 中文查詢若有多筆候選，列出前 10 筆
		if _, err := strconv.Atoi(itemArg); err != nil {
			candidates := deps.Items.FindByName(itemArg)
			if len(candidates) > 1 {
				gmMsgf(sess, "\\f3找到 %d 筆，請更精確：", len(candidates))
				limit := len(candidates)
				if limit > 10 {
					limit = 10
				}
				for i := 0; i < limit; i++ {
					gmMsgf(sess, "  %s (id=%d)", candidates[i].Name, candidates[i].ItemID)
				}
				return
			}
		}
		gmMsgf(sess, "\\f3找不到物品: %s", itemArg)
		return
	}

	canEnchant := itemInfo.Category == data.CategoryWeapon || itemInfo.Category == data.CategoryArmor
	if enchantSpecified && !canEnchant {
		gmMsg(sess, "\\f3只有武器或防具可以指定強化值")
		return
	}
	if !canEnchant {
		enchant = 0
	}
	if !itemInfo.Stackable && itemInfo.ItemID != world.AdenaItemID && count > 10 {
		gmMsg(sess, "\\f3不可以堆疊的物品一次創造數量禁止超過10")
		return
	}

	stackable := itemInfo.Stackable || itemInfo.ItemID == world.AdenaItemID
	existing := player.Inv.FindByItemID(itemInfo.ItemID)
	if stackable && existing != nil && int64(existing.Count)+int64(count) > 2147483647 {
		gmMsg(sess, "堆疊數量超出可儲存的範圍。")
		return
	}
	neededSlots := 1
	if stackable && existing != nil {
		neededSlots = 0
	} else if !stackable {
		neededSlots = int(count)
	}
	if player.Inv.Size()+neededSlots > world.MaxInventorySize {
		gmMsg(sess, "\\f3背包已滿")
		return
	}
	before := gmInventoryItemCount(player, itemInfo.ItemID)

	deps.GMCmd.GiveItem(sess, player, itemInfo.ItemID, count, enchant)
	delivered := gmInventoryItemCount(player, itemInfo.ItemID) - before
	if delivered <= 0 {
		gmMsg(sess, "物品未加入背包，請確認負重與容量。")
		return
	}

	name := itemInfo.Name
	if enchant > 0 {
		name = fmt.Sprintf("+%d %s", enchant, name)
	}
	gmMsgf(sess, "已給予 %s x%d", name, delivered)
}

func gmInventoryItemCount(p *world.PlayerInfo, id int32) int64 {
	var count int64
	for _, item := range p.Inv.Items {
		if item.ItemID == id {
			count += int64(item.Count)
		}
	}
	return count
}

func parseGMItemArgs(args []string) (itemArg string, count int32, enchant int8, enchantSpecified bool, errMsg string) {
	count = 1
	itemEnd := len(args)

	switch {
	case len(args) >= 3 && isGMItemSignedInt(args[len(args)-2]) && isGMItemPositiveInt(args[len(args)-1]):
		e, ok := parseGMItemEnchant(args[len(args)-2])
		if !ok {
			return "", 0, 0, false, "無效的強化值"
		}
		c, _ := strconv.Atoi(args[len(args)-1])
		enchant = e
		count = int32(c)
		enchantSpecified = true
		itemEnd = len(args) - 2

	case len(args) >= 3 && isGMItemPositiveInt(args[len(args)-2]) && isGMItemSignedInt(args[len(args)-1]):
		c, _ := strconv.Atoi(args[len(args)-2])
		e, ok := parseGMItemEnchant(args[len(args)-1])
		if !ok {
			return "", 0, 0, false, "無效的強化值"
		}
		count = int32(c)
		enchant = e
		enchantSpecified = true
		itemEnd = len(args) - 2

	case len(args) >= 3 && isGMItemPositiveInt(args[len(args)-2]) && isGMItemPlainInt(args[len(args)-1]):
		c, _ := strconv.Atoi(args[len(args)-2])
		e, ok := parseGMItemEnchant(args[len(args)-1])
		if !ok {
			return "", 0, 0, false, "無效的強化值"
		}
		count = int32(c)
		enchant = e
		enchantSpecified = true
		itemEnd = len(args) - 2

	case len(args) >= 2 && isGMItemSignedInt(args[len(args)-1]):
		e, ok := parseGMItemEnchant(args[len(args)-1])
		if !ok {
			return "", 0, 0, false, "無效的強化值"
		}
		enchant = e
		enchantSpecified = true
		itemEnd = len(args) - 1

	case len(args) >= 2 && isGMItemPositiveInt(args[len(args)-1]):
		c, _ := strconv.Atoi(args[len(args)-1])
		count = int32(c)
		itemEnd = len(args) - 1
	}

	if itemEnd <= 0 {
		return "", 0, 0, false, "缺少物品名稱或 ID"
	}
	if count <= 0 {
		return "", 0, 0, false, "無效的物品數量"
	}
	return strings.Join(args[:itemEnd], " "), count, enchant, enchantSpecified, ""
}

func parseGMItemEnchant(s string) (int8, bool) {
	v, err := strconv.Atoi(s)
	if err != nil || v < -127 || v > 127 {
		return 0, false
	}
	return int8(v), true
}

func isGMItemSignedInt(s string) bool {
	if len(s) < 2 || (s[0] != '+' && s[0] != '-') {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

func isGMItemPositiveInt(s string) bool {
	if !isGMItemPlainInt(s) {
		return false
	}
	v, _ := strconv.Atoi(s)
	return v > 0 && int64(v) <= 2147483647
}

func isGMItemPlainInt(s string) bool {
	if s == "" || s[0] == '+' || s[0] == '-' {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

// resolveItemArg 將 .item 第一個參數解析為 ItemInfo。
// 數字 → 直接 Get(id)；非數字 → FindByName 完全相符優先，僅單筆時回傳。
func resolveItemArg(deps *Deps, arg string) *data.ItemInfo {
	if id, err := strconv.Atoi(arg); err == nil {
		if id <= 0 || int64(id) > 2147483647 {
			return nil
		}
		return deps.Items.Get(int32(id))
	}
	results := deps.Items.FindByName(arg)
	if len(results) == 1 {
		return results[0]
	}
	return nil
}

func gmGold(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .gold <數量>")
		return
	}
	amount, err := strconv.Atoi(args[0])
	if err != nil || amount <= 0 || int64(amount)+int64(player.Inv.GetAdena()) > 2147483647 {
		gmMsg(sess, "\\f3無效的金幣數量")
		return
	}

	before := player.Inv.GetAdena()
	deps.GMCmd.GiveGold(sess, player, int32(amount))
	if player.Inv.GetAdena() <= before {
		gmMsg(sess, "金幣未加入背包，請確認容量。")
		return
	}

	gmMsgf(sess, "已給予 %d 金幣 (持有: %d)", amount, player.Inv.GetAdena())
}

func gmLawful(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if deps == nil || deps.GMCmd == nil {
		gmMsg(sess, "\\f3GM 指令系統未初始化")
		return
	}
	if len(args) < 1 || len(args) > 2 {
		gmMsg(sess, "\\f3用法: .lawful <+/-數值> [角色名稱]")
		return
	}
	arg := args[0]
	if len(arg) < 2 || (arg[0] != '+' && arg[0] != '-') {
		gmMsg(sess, "\\f3用法: .lawful <+/-數值> [角色名稱]")
		return
	}
	delta64, err := strconv.ParseInt(arg, 10, 32)
	if err != nil {
		gmMsg(sess, "\\f3正義值調整量必須是 -2147483648 到 2147483647 之間的整數")
		return
	}

	target := player
	if len(args) == 2 && !strings.EqualFold(args[1], "me") {
		if deps.World == nil {
			gmMsg(sess, "\\f3世界狀態尚未初始化，無法指定角色")
			return
		}
		target = deps.World.GetByName(args[1])
		if target == nil {
			gmMsgf(sess, "\\f3找不到線上角色: %s", args[1])
			return
		}
	}

	targetSess := sess
	if target.Session != nil {
		targetSess = target.Session
	}
	deps.GMCmd.AdjustLawful(targetSess, target, int32(delta64))
	gmMsgf(sess, "%s 正義值已調整 %+d，目前 %d", target.Name, int32(delta64), target.Lawful)
}

func gmSpell(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .spell <skillID>  (0 = 學全部)")
		return
	}
	skillID, err := strconv.Atoi(args[0])
	if err != nil {
		gmMsg(sess, "\\f3無效的技能ID")
		return
	}

	if skillID == 0 {
		// Learn all skills
		count := 0
		for id := int32(1); id <= 256; id++ {
			sk := deps.Skills.Get(id)
			if sk == nil {
				continue
			}
			// Check if already known
			known := false
			for _, s := range player.KnownSpells {
				if s == id {
					known = true
					break
				}
			}
			if !known {
				player.KnownSpells = append(player.KnownSpells, id)
				count++
			}
		}
		// Send full skill list
		player.Dirty = true
		sendAllSpells(sess, player, deps)
		gmMsgf(sess, "已學會全部技能 (新增 %d 個)", count)
		return
	}

	sk := deps.Skills.Get(int32(skillID))
	if sk == nil {
		gmMsgf(sess, "\\f3找不到技能: %d", skillID)
		return
	}

	// Check if already known
	for _, s := range player.KnownSpells {
		if s == int32(skillID) {
			gmMsgf(sess, "已經學會技能: %s (ID:%d)", sk.Name, skillID)
			return
		}
	}

	player.KnownSpells = append(player.KnownSpells, int32(skillID))
	player.Dirty = true

	// Send updated skill list
	sendAllSpells(sess, player, deps)
	gmMsgf(sess, "已學會技能: %s (ID:%d)", sk.Name, skillID)
}

// sendAllSpells re-sends the complete spell list to the client.
func sendAllSpells(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	if deps.Skills == nil {
		return
	}
	var spells []*data.SkillInfo
	for _, sid := range player.KnownSpells {
		if sk := deps.Skills.Get(sid); sk != nil {
			spells = append(spells, sk)
		}
	}
	sendSkillList(sess, spells)
}

func gmAllSkill(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	if player.ClassType < 0 || player.ClassType > 6 || deps.Skills == nil {
		gmMsg(sess, "\\f3未知的職業類型")
		return
	}

	// Build set of already known spells
	knownSet := make(map[int32]bool, len(player.KnownSpells))
	for _, sid := range player.KnownSpells {
		knownSet[sid] = true
	}

	count := 0
	for _, sk := range deps.Skills.All() {
		if sk.Name == "none" || sk.Name == "" {
			continue
		}
		if !gmClassCanLearnSkill(player.ClassType, sk.SkillID) {
			continue
		}
		if knownSet[sk.SkillID] {
			continue
		}
		player.KnownSpells = append(player.KnownSpells, sk.SkillID)
		knownSet[sk.SkillID] = true
		count++
	}

	player.Dirty = true
	sendAllSpells(sess, player, deps)
	BroadcastToPlayers(deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID), BuildSkillEffect(player.CharID, 227))

	classNames := []string{"王族", "騎士", "精靈", "法師", "黑暗精靈", "龍騎士", "幻術師"}
	gmMsgf(sess, "已學會 %s 全部技能 (新增 %d 個)", classNames[player.ClassType], count)
}

// Java L1AddSkill includes basic magic for royal/knight/elf/dark elf.
func gmClassCanLearnSkill(class int16, id int32) bool {
	switch class {
	case 0:
		return id >= 1 && id <= 16 || id >= 113 && id <= 120
	case 1:
		return id >= 1 && id <= 8 || id >= 87 && id <= 91
	case 2:
		return id >= 1 && id <= 48 || id >= 129 && id <= 176
	case 3:
		return id >= 1 && id <= 80
	case 4:
		return id >= 1 && id <= 16 || id >= 97 && id <= 111
	case 5:
		return id >= 181 && id <= 195
	case 6:
		return id >= 201 && id <= 220
	default:
		return false
	}
}

func gmSpawn(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 || len(args) > 3 || deps.Npcs == nil {
		gmMsg(sess, "用法：.spawn <NPC ID|名稱> [數量 1-500] [範圍 0-100]")
		return
	}
	var npcID int32
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err == nil {
		npcID = int32(id)
	} else if npc := deps.Npcs.FindByNameWithoutSpace(args[0]); npc != nil {
		npcID = npc.NpcID
	}
	tmpl := deps.Npcs.Get(npcID)
	if tmpl == nil {
		gmMsg(sess, "找不到符合條件的 NPC。")
		return
	}
	count, radius := 1, 0
	if len(args) >= 2 {
		count, err = strconv.Atoi(args[1])
		if err != nil || count < 1 || count > 500 {
			gmMsg(sess, "數量必須介於 1-500。")
			return
		}
	}
	if len(args) >= 3 {
		radius, err = strconv.Atoi(args[2])
		if err != nil || radius < 0 || radius > 100 {
			gmMsg(sess, "範圍必須介於 0-100。")
			return
		}
	}
	spawner, ok := deps.GMCmd.(interface {
		SpawnGMNPC(*world.PlayerInfo, int32, int, int) (int, error)
	})
	if !ok {
		gmMsg(sess, "NPC 生成系統未初始化。")
		return
	}
	spawned, err := spawner.SpawnGMNPC(player, npcID, count, radius)
	if err != nil {
		gmMsgf(sess, "召怪失敗：%v", err)
		return
	}
	gmMsgf(sess, "已召喚 %s (ID:%d) x%d，範圍:%d", tmpl.Name, npcID, spawned, radius)
}

func gmKill(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	// Kill nearby NPCs within 3 tiles
	nearby := deps.World.GetNearbyNpcsInShow(player.X, player.Y, player.MapID, player.ShowID)
	killed := 0
	for _, npc := range nearby {
		if npc.Dead {
			continue
		}
		dx := player.X - npc.X
		dy := player.Y - npc.Y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		dist := dx
		if dy > dist {
			dist = dy
		}
		if dist <= 3 {
			npc.HP = 0
			npc.Dead = true
			deps.World.NpcDied(npc)
			viewers := deps.World.GetNearbyPlayersInShow(npc.X, npc.Y, npc.MapID, 0, npc.ShowID)
			for _, v := range viewers {
				sendActionGfx(v.Session, npc.ID, 8)
				SendNpcDeadPack(v.Session, npc)
			}
			npc.DeleteTimer = 50 // 10 seconds for death animation
			if npc.RespawnDelay > 0 {
				npc.RespawnTimer = npc.RespawnDelay * 5
			}
			killed++
		}
	}
	gmMsgf(sess, "已擊殺 %d 個NPC", killed)
}

func gmKillAll(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	nearby := deps.World.GetNearbyNpcsInShow(player.X, player.Y, player.MapID, player.ShowID)
	killed := 0
	for _, npc := range nearby {
		if npc.Dead {
			continue
		}
		npc.HP = 0
		npc.Dead = true
		deps.World.NpcDied(npc)
		viewers := deps.World.GetNearbyPlayersInShow(npc.X, npc.Y, npc.MapID, 0, npc.ShowID)
		for _, v := range viewers {
			sendActionGfx(v.Session, npc.ID, 8)
			SendNpcDeadPack(v.Session, npc)
		}
		npc.DeleteTimer = 50 // 10 seconds for death animation
		if npc.RespawnDelay > 0 {
			npc.RespawnTimer = npc.RespawnDelay * 5
		}
		killed++
	}
	gmMsgf(sess, "已擊殺附近 %d 個NPC", killed)
}

func gmSpeed(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .speed <0-4>  (0=正常,1=加速,2=勇水,3=巧克力蛋糕,4=精靈餅乾)")
		return
	}
	spd, err := strconv.Atoi(args[0])
	if err != nil || spd < 0 || spd > 4 {
		gmMsg(sess, "\\f3速度必須在 0-4 之間")
		return
	}

	// 先清除舊狀態
	player.MoveSpeed = 0
	player.BraveSpeed = 0
	player.HasteTicks = 0
	player.BraveTicks = 0
	clearThird := player.HasBuff(SkillStatusThirdSpeed)
	if clearThird {
		player.RemoveBuff(SkillStatusThirdSpeed)
		sess.Send(BuildLiquor(player.CharID, 0))
	}

	const dur = 3600       // 封包中的秒數
	const ticks = 3600 * 5 // 內部 tick 數（1小時）

	switch spd {
	case 0:
		// 取消全部：分別發送 haste 和 brave 取消封包
		sendSpeedPacket(sess, player.CharID, 0, 0)
		sendBravePacket(sess, player.CharID, 0, 0)
	case 1:
		// 一段加速（移動加速）
		player.MoveSpeed = 1
		player.HasteTicks = ticks
		sendSpeedPacket(sess, player.CharID, 1, dur)
		sendBravePacket(sess, player.CharID, 0, 0)
	case 2:
		// 二段加速（移動 + 勇敢藥水）
		player.MoveSpeed = 1
		player.BraveSpeed = 1
		player.HasteTicks = ticks
		player.BraveTicks = ticks
		sendSpeedPacket(sess, player.CharID, 1, dur)
		sendBravePacket(sess, player.CharID, 1, dur)
	case 3:
		// 巧克力蛋糕（移動 + 超級勇敢 braveSpeed=5）
		player.MoveSpeed = 1
		player.BraveSpeed = 5
		player.HasteTicks = ticks
		player.BraveTicks = ticks
		sendSpeedPacket(sess, player.CharID, 1, dur)
		sendBravePacket(sess, player.CharID, 5, dur)
	case 4:
		// 精靈餅乾（移動 + 精靈勇敢 braveSpeed=3）
		player.MoveSpeed = 1
		player.BraveSpeed = 3
		player.HasteTicks = ticks
		player.BraveTicks = ticks
		sendSpeedPacket(sess, player.CharID, 1, dur)
		sendBravePacket(sess, player.CharID, 3, dur)
	}

	// 廣播給附近玩家
	nearby := deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, sess.ID, player.ShowID)
	for _, other := range nearby {
		if !other.CanSeePlayer(player) {
			continue
		}
		sendSpeedPacket(other.Session, player.CharID, player.MoveSpeed, 0)
		sendBravePacket(other.Session, player.CharID, player.BraveSpeed, 0)
		if clearThird {
			other.Session.Send(BuildLiquor(player.CharID, 0))
		}
	}

	// 更新角色狀態（讓客戶端 buff 圖標正確顯示）
	SendPlayerStatus(sess, player)

	names := []string{"正常", "加速", "勇敢藥水", "巧克力蛋糕", "精靈餅乾"}
	gmMsgf(sess, "移動速度已設為: %s", names[spd])
}

func gmWho(sess *net.Session, deps *Deps) {
	count := 0
	deps.World.AllPlayers(func(p *world.PlayerInfo) {
		count++
		gmMsgf(sess, "  %s (Lv.%d) 位置:(%d,%d) 地圖:%d", p.Name, p.Level, p.X, p.Y, p.MapID)
	})
	gmMsgf(sess, "線上人數: %d", count)
}

func gmGoto(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .goto <玩家名>")
		return
	}
	target := deps.World.GetByName(args[0])
	if target == nil {
		gmMsgf(sess, "\\f3找不到玩家: %s", args[0])
		return
	}

	teleportPlayerInShow(sess, player, target.X, target.Y, target.MapID, 5, target.ShowID, deps)
	gmMsgf(sess, "已傳送至 %s 身邊 (%d,%d) 地圖:%d", target.Name, target.X, target.Y, target.MapID)
}

func gmRecall(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .recall <玩家名>")
		return
	}
	target := deps.World.GetByName(args[0])
	if target == nil {
		gmMsgf(sess, "\\f3找不到玩家: %s", args[0])
		return
	}

	teleportPlayerInShow(target.Session, target, player.X, player.Y, player.MapID, 5, player.ShowID, deps)
	gmMsgf(sess, "已召喚 %s 到身邊", target.Name)
	gmMsg(target.Session, "你被GM召喚了")
}

func gmExp(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .exp <數值>")
		return
	}
	val, err := strconv.Atoi(args[0])
	if err != nil || val <= 0 || int64(val) > 2147483647 {
		gmMsg(sess, "\\f3無效的經驗值")
		return
	}

	if int64(player.Exp)+int64(val) > 2147483647 {
		gmMsg(sess, "經驗值加總超出角色可儲存的範圍。")
		return
	}
	deps.Combat.AddExp(player, int32(val))
	gmMsgf(sess, "已獲得 %d 經驗值 (Lv.%d Exp:%d)", val, player.Level, player.Exp)
}

func gmClass(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .class <0-6>")
		gmMsg(sess, "  0=王族 1=騎士 2=精靈 3=法師 4=黑暗精靈 5=龍騎士 6=幻術師")
		return
	}
	classType, err := strconv.Atoi(args[0])
	if err != nil || classType < 0 || classType > 6 {
		gmMsg(sess, "\\f3職業必須在 0-6 之間")
		return
	}

	if deps.CharRepo == nil {
		gmMsg(sess, "角色資料庫未初始化，未變更職業。")
		return
	}
	if err := changeGMClass(player, int16(classType), deps, deps.CharRepo); err != nil {
		gmMsgf(sess, "職業變更失敗：%v", err)
		return
	}
	sendPlayerStatus(sess, player)
	broadcastVisualUpdate(sess, player, deps)
	sendOwnCharPackPlayer(sess, player)

	names := []string{"王族", "騎士", "精靈", "法師", "黑暗精靈", "龍騎士", "幻術師"}
	gmMsgf(sess, "職業已變更為: %s", names[classType])
}

func gmRez(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	var target *world.PlayerInfo
	if len(args) >= 1 {
		target = deps.World.GetByName(args[0])
		if target == nil {
			gmMsgf(sess, "\\f3找不到玩家: %s", args[0])
			return
		}
	} else {
		target = player
	}

	if !target.Dead {
		gmMsgf(sess, "%s 沒有死亡", target.Name)
		return
	}

	target.Dead = false
	target.HP = target.MaxHP
	target.MP = target.MaxMP
	target.Dirty = true
	target.LastMoveTime = 0
	if deps.Death != nil {
		deps.Death.ClearPlayerTomb(target)
	}
	deps.World.OccupyEntity(target.MapID, target.X, target.Y, target.CharID)

	sendHpUpdate(target.Session, target)
	sendMpUpdate(target.Session, target)
	sendPlayerStatus(target.Session, target)

	resData := BuildResurrection(target, player.CharID, 0)
	target.Session.Send(resData)
	sendOwnCharPackPlayer(target.Session, target)
	nearby := deps.World.GetNearbyPlayersInShow(target.X, target.Y, target.MapID, target.SessionID, target.ShowID)
	for _, viewer := range nearby {
		if viewer.CanSeePlayer(target) {
			viewer.Session.Send(resData)
			SendPutObject(viewer.Session, target)
		}
	}

	if target == player {
		gmMsg(sess, "已復活")
	} else {
		gmMsgf(sess, "已復活 %s", target.Name)
		gmMsg(target.Session, "你被GM復活了")
	}
}

func gmShowInfo(sess *net.Session, player *world.PlayerInfo) {
	gmMsgf(sess, "=== %s 角色資訊 ===", player.Name)
	gmMsgf(sess, "等級:%d 職業:%d 經驗:%d", player.Level, player.ClassType, player.Exp)
	gmMsgf(sess, "HP:%d/%d MP:%d/%d AC:%d MR:%d", player.HP, player.MaxHP, player.MP, player.MaxMP, player.AC, player.MR)
	gmMsgf(sess, "STR:%d DEX:%d CON:%d WIS:%d INT:%d CHA:%d", player.Str, player.Dex, player.Con, player.Wis, player.Intel, player.Cha)
	gmMsgf(sess, "位置:(%d,%d) 地圖:%d 朝向:%d", player.X, player.Y, player.MapID, player.Heading)
	gmMsgf(sess, "命中:%d 傷害:%d 弓命中:%d 弓傷害:%d", player.HitMod, player.DmgMod, player.BowHitMod, player.BowDmgMod)
	gmMsgf(sess, "SP:%d HPR:%d MPR:%d Dodge:%d", player.SP, player.HPR, player.MPR, player.Dodge)
	gmMsgf(sess, "火抗:%d 水抗:%d 風抗:%d 地抗:%d", player.FireRes, player.WaterRes, player.WindRes, player.EarthRes)
	gmMsgf(sess, "背包物品: %d/%d", player.Inv.Size(), world.MaxInventorySize)
}

// calcBaseHPMP estimates HP/MP for a given level using Lua formulas.
func calcBaseHPMP(classType, level, con, wis int16, deps *Deps) (int32, int32) {
	// Get starting HP/MP from Lua character creation data
	initHP := int32(deps.Scripting.CalcInitHP(int(classType), int(con)))
	initMP := int32(deps.Scripting.CalcInitMP(int(classType), int(wis)))

	baseHP := initHP
	baseMP := initMP
	for lv := int16(2); lv <= level; lv++ {
		result := deps.Scripting.CalcLevelUp(int(classType), int(con), int(wis))
		baseHP += int32(result.HP)
		baseMP += int32(result.MP)
	}

	return baseHP, baseMP
}

func gmPoly(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	// Java orders the target before the ID; retain the existing Go ID-first form.
	if len(args) == 2 {
		if _, err := strconv.Atoi(args[0]); err != nil {
			args = []string{args[1], args[0]}
		}
	}
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .poly <polyID> [玩家名]")
		return
	}
	polyID, err := strconv.Atoi(args[0])
	if err != nil || polyID <= 0 {
		gmMsg(sess, "\\f3無效的變身ID")
		return
	}

	target := player
	if len(args) >= 2 && !strings.EqualFold(args[1], "me") {
		target = deps.World.GetByName(args[1])
		if target == nil {
			gmMsgf(sess, "\\f3找不到玩家: %s", args[1])
			return
		}
	}

	if deps.Polys == nil {
		gmMsg(sess, "\\f3變身資料未載入")
		return
	}

	poly := deps.Polys.GetByID(int32(polyID))
	if poly == nil {
		gmMsgf(sess, "\\f3找不到變身形態: %d", polyID)
		return
	}

	if deps.Polymorph != nil {
		deps.Polymorph.DoPoly(target, int32(polyID), 7200, data.PolyCauseGM)
	}
	gmMsgf(sess, "已將 %s 變身為 %s (GFX:%d)", target.Name, poly.Name, polyID)
}

// gmPolyGfx directly changes the player's sprite to any GFX ID, bypassing
// the polymorph data table. Usage: .polygfx <gfxID> [玩家名]
// Revert with .undopoly.
func gmPolyGfx(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .polygfx <gfxID> [玩家名]")
		return
	}
	gfxID, err := strconv.Atoi(args[0])
	if err != nil || gfxID <= 0 || gfxID > 65535 {
		gmMsg(sess, "\\f3無效的圖檔ID (1-65535)")
		return
	}

	target := player
	if len(args) >= 2 {
		target = deps.World.GetByName(args[1])
		if target == nil {
			gmMsgf(sess, "\\f3找不到玩家: %s", args[1])
			return
		}
	}

	// If already polymorphed, revert first
	if target.TempCharGfx > 0 && deps.Polymorph != nil {
		deps.Polymorph.UndoPoly(target)
	}

	target.TempCharGfx = int32(gfxID)
	target.PolyID = 0 // no equip restrictions for raw GFX change

	// Broadcast S_ChangeShape to self + nearby
	nearby := deps.World.GetNearbyPlayersInShow(target.X, target.Y, target.MapID, 0, target.ShowID)
	for _, viewer := range nearby {
		if !viewer.CanSeePlayer(target) {
			continue
		}
		sendChangeShape(viewer.Session, target.CharID, int32(gfxID), target.CurrentWeapon)
	}

	gmMsgf(sess, "已將 %s 變身為 GFX:%d", target.Name, gfxID)
}

func gmUndoPoly(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	target := player
	if len(args) >= 1 {
		target = deps.World.GetByName(args[0])
		if target == nil {
			gmMsgf(sess, "\\f3找不到玩家: %s", args[0])
			return
		}
	}

	if target.TempCharGfx == 0 {
		gmMsgf(sess, "%s 沒有變身", target.Name)
		return
	}

	if deps.Polymorph != nil {
		deps.Polymorph.UndoPoly(target)
	}
	if target == player {
		gmMsg(sess, "已解除變身")
	} else {
		gmMsgf(sess, "已解除 %s 的變身", target.Name)
	}
}

func gmLoc(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	target := player
	if len(args) >= 1 {
		target = deps.World.GetByName(args[0])
		if target == nil {
			gmMsgf(sess, "\\f3找不到玩家: %s", args[0])
			return
		}
	}
	gmMsgf(sess, "[%s] 座標: (%d, %d)  地圖: %d  朝向: %d",
		target.Name, target.X, target.Y, target.MapID, target.Heading)
}

// gmWall creates a collision wall (door) at the facing tile for testing.
// Usage: .wall [mode]
//
//	mode 1 (default): S_DoorPack(GfxId=0) + S_CHANGE_ATTR + S_REMOVE_OBJECT (invisible test)
//	mode 2: S_CHANGE_ATTR only (no door object)
//	mode 3: S_DoorPack(GfxId=0) + S_CHANGE_ATTR, keep visible (no remove)
func gmWall(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	mode := 1
	if len(args) > 0 {
		if m, err := strconv.Atoi(args[0]); err == nil && m >= 1 && m <= 6 {
			mode = m
		}
	}

	h := player.Heading
	if h < 0 || h > 7 {
		h = 0
	}
	tx := player.X + headingDX[h]
	ty := player.Y + headingDY[h]

	switch mode {
	case 1:
		// Door + S_CHANGE_ATTR + immediate S_REMOVE_OBJECT (try to keep passability but hide visual)
		door := &world.DoorInfo{
			ID: world.NextDoorID(), GfxID: 0, X: tx, Y: ty, MapID: player.MapID,
			MaxHP: 0, HP: 1, Direction: 0, LeftEdge: tx, RightEdge: tx, OpenStatus: world.DoorActionClose,
		}
		door2 := &world.DoorInfo{
			ID: world.NextDoorID(), GfxID: 0, X: tx, Y: ty, MapID: player.MapID,
			MaxHP: 0, HP: 1, Direction: 1, LeftEdge: ty, RightEdge: ty, OpenStatus: world.DoorActionClose,
		}
		deps.World.AddDoor(door)
		deps.World.AddDoor(door2)
		SendDoorPerceive(sess, door)
		SendDoorPerceive(sess, door2)
		// Immediately remove the visual objects — passability might persist
		SendRemoveObject(sess, door.ID)
		SendRemoveObject(sess, door2.ID)
		gmMsgf(sess, "模式1: 門+封包+移除視覺 (%d,%d)", tx, ty)

	case 2:
		// S_CHANGE_ATTR only (no door object)
		sendDoorAttr(sess, tx, ty, 0, false)
		sendDoorAttr(sess, tx, ty, 1, false)
		sendDoorAttr(sess, tx, ty+1, 0, false)
		sendDoorAttr(sess, tx-1, ty, 1, false)
		gmMsgf(sess, "模式2: 僅S_CHANGE_ATTR (%d,%d) 無門物件", tx, ty)

	case 3:
		// Door + S_CHANGE_ATTR, keep visible (no S_REMOVE_OBJECT) — same as old mode 1
		door := &world.DoorInfo{
			ID: world.NextDoorID(), GfxID: 0, X: tx, Y: ty, MapID: player.MapID,
			MaxHP: 0, HP: 1, Direction: 0, LeftEdge: tx, RightEdge: tx, OpenStatus: world.DoorActionClose,
		}
		door2 := &world.DoorInfo{
			ID: world.NextDoorID(), GfxID: 0, X: tx, Y: ty, MapID: player.MapID,
			MaxHP: 0, HP: 1, Direction: 1, LeftEdge: ty, RightEdge: ty, OpenStatus: world.DoorActionClose,
		}
		deps.World.AddDoor(door)
		deps.World.AddDoor(door2)
		SendDoorPerceive(sess, door)
		SendDoorPerceive(sess, door2)
		gmMsgf(sess, "模式3: 門+封包 保留視覺 (%d,%d) ID=%d,%d", tx, ty, door.ID, door2.ID)

	case 4:
		// Only S_DoorPack (no S_CHANGE_ATTR) + S_REMOVE_OBJECT — test if DoorPack alone blocks
		door := &world.DoorInfo{
			ID: world.NextDoorID(), GfxID: 0, X: tx, Y: ty, MapID: player.MapID,
			MaxHP: 0, HP: 1, Direction: 0, LeftEdge: tx, RightEdge: tx, OpenStatus: world.DoorActionClose,
		}
		deps.World.AddDoor(door)
		sendDoorPack(sess, door)
		SendRemoveObject(sess, door.ID)
		gmMsgf(sess, "模式4: 僅S_DoorPack+移除 無S_CHANGE_ATTR (%d,%d)", tx, ty)

	case 5:
		// S_CHANGE_ATTR comprehensive — block ALL surrounding edges
		// Block tile itself (both directions)
		sendDoorAttr(sess, tx, ty, 0, false)
		sendDoorAttr(sess, tx, ty, 1, false)
		// Block all 4 adjacent tiles' edges pointing toward (tx, ty)
		sendDoorAttr(sess, tx, ty+1, 0, false) // south tile "/" edge
		sendDoorAttr(sess, tx, ty+1, 1, false) // south tile "\" edge
		sendDoorAttr(sess, tx-1, ty, 0, false) // west tile "/" edge
		sendDoorAttr(sess, tx-1, ty, 1, false) // west tile "\" edge
		sendDoorAttr(sess, tx, ty-1, 0, false) // north tile "/" edge
		sendDoorAttr(sess, tx, ty-1, 1, false) // north tile "\" edge
		sendDoorAttr(sess, tx+1, ty, 0, false) // east tile "/" edge
		sendDoorAttr(sess, tx+1, ty, 1, false) // east tile "\" edge
		gmMsgf(sess, "模式5: 全方位S_CHANGE_ATTR (%d,%d) + 4鄰居", tx, ty)

	case 6:
		// Try S_DoorPack with dead status (37) + S_REMOVE_OBJECT — dead door might be invisible
		door := &world.DoorInfo{
			ID: world.NextDoorID(), GfxID: 0, X: tx, Y: ty, MapID: player.MapID,
			MaxHP: 0, HP: 1, Direction: 0, LeftEdge: tx, RightEdge: tx, OpenStatus: world.DoorActionClose,
		}
		door2 := &world.DoorInfo{
			ID: world.NextDoorID(), GfxID: 0, X: tx, Y: ty, MapID: player.MapID,
			MaxHP: 0, HP: 1, Direction: 1, LeftEdge: ty, RightEdge: ty, OpenStatus: world.DoorActionClose,
		}
		deps.World.AddDoor(door)
		deps.World.AddDoor(door2)
		// Send door pack + S_CHANGE_ATTR
		SendDoorPerceive(sess, door)
		SendDoorPerceive(sess, door2)
		// Then send "die" action to make them disappear visually
		sendDoorAction(sess, door.ID, world.DoorActionDie)
		sendDoorAction(sess, door2.ID, world.DoorActionDie)
		gmMsgf(sess, "模式6: 門+死亡動畫 (%d,%d) ID=%d,%d", tx, ty, door.ID, door2.ID)
	}
}

// gmClearWall removes all test walls/doors near the player.
func gmClearWall(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	removed := 0
	nearbyDoors := deps.World.GetNearbyDoors(player.X, player.Y, player.MapID)
	for _, d := range nearbyDoors {
		// Only remove GM-spawned doors (GfxId=0 or test doors at exact position)
		if d.GfxID == 0 || d.GfxID == 2618 {
			SendRemoveObject(sess, d.ID)
			// Make passable again
			sendDoorAttr(sess, d.EntranceX(), d.EntranceY(), d.Direction, true)
			deps.World.RemoveDoor(d.ID)
			removed++
		}
	}
	gmMsgf(sess, "已清除 %d 個測試牆壁", removed)
}

// gmTime 顯示或設定遊戲時間。
// 用法: .time          — 查看當前遊戲時間
//
//	.time set <小時>  — 強制設定遊戲時間（0-23）
func gmTime(sess *net.Session, _ *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) >= 2 && strings.ToLower(args[0]) == "set" {
		hour, err := strconv.Atoi(args[1])
		if err != nil || hour < 0 || hour > 23 {
			gmMsg(sess, "\\f3用法: .time set <0-23>")
			return
		}
		world.SetGameTimeOffset(hour)
		// 廣播新的 S_GameTime 給所有在線玩家
		gt := world.GameTimeNow()
		deps.World.AllPlayers(func(p *world.PlayerInfo) {
			sendGameTime(p.Session, gt.Seconds())
		})
		dayNight := "白天"
		if gt.IsNight() {
			dayNight = "夜晚"
		}
		gmMsgf(sess, "遊戲時間已設為 %02d:%02d (%s)", gt.Hour(), gt.Minute(), dayNight)
		return
	}

	gt := world.GameTimeNow()
	dayNight := "白天"
	if gt.IsNight() {
		dayNight = "夜晚"
	}
	gmMsgf(sess, "遊戲時間: %02d:%02d (%s)", gt.Hour(), gt.Minute(), dayNight)
}

func gmWeather(sess *net.Session, _ *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, ".weather <0-4, 16-19>  (0/4=clear, 1-3=snow, 16=stop rain, 17-19=rain)")
		return
	}
	val, err := strconv.Atoi(args[0])
	if err != nil {
		gmMsg(sess, ".weather <0-4, 16-19>")
		return
	}
	// Java Weather.java 的雨雪代碼，加上 L1World 的初始值 4。
	valid := false
	for _, t := range []int{0, 1, 2, 3, 4, 16, 17, 18, 19} {
		if val == t {
			valid = true
			break
		}
	}
	if !valid {
		gmMsg(sess, "有效值: 0,1,2,3,4,16,17,18,19")
		return
	}
	deps.World.Weather = byte(val)
	deps.World.AllPlayers(func(p *world.PlayerInfo) {
		sendWeather(p.Session, byte(val))
	})
	gmMsgf(sess, "天氣已變更為 %d", val)
}

// gmStressTest 一次生成大量怪物用於壓力測試。
// 用法: .stresstest <npcID> [數量] [半徑]
// 怪物分散在玩家周圍，不會重生（關服即消失）。
func gmStressTest(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) < 1 {
		gmMsg(sess, "\\f3用法: .stresstest <npcID> [數量] [半徑]")
		return
	}

	npcID, err := strconv.Atoi(args[0])
	if err != nil {
		gmMsg(sess, "\\f3無效的NPC ID")
		return
	}

	count := 10000
	if len(args) >= 2 {
		if c, err := strconv.Atoi(args[1]); err == nil && c > 0 {
			if c > 10000 {
				c = 10000
			}
			count = c
		}
	}

	radius := int32(50)
	if len(args) >= 3 {
		if r, err := strconv.Atoi(args[2]); err == nil && r > 0 {
			if r > 100 {
				r = 100
			}
			radius = int32(r)
		}
	}

	if deps.Npcs == nil {
		gmMsg(sess, "\\f3NPC模板未載入")
		return
	}
	tmpl := deps.Npcs.Get(int32(npcID))
	if tmpl == nil {
		gmMsgf(sess, "\\f3找不到NPC模板: %d", npcID)
		return
	}

	// 查詢動畫速度（只查一次，所有 NPC 共用）
	atkSpeed := tmpl.AtkSpeed
	moveSpeed := tmpl.PassiveSpeed
	if deps.SprTable != nil {
		gfx := int(tmpl.GfxID)
		if tmpl.AtkSpeed != 0 {
			if v := deps.SprTable.GetAttackSpeed(gfx, data.ActAttack); v > 0 {
				atkSpeed = int16(v)
			}
		}
		if tmpl.PassiveSpeed != 0 {
			if v := deps.SprTable.GetMoveSpeed(gfx, data.ActWalk); v > 0 {
				moveSpeed = int16(v)
			}
		}
	}

	gmMsgf(sess, "開始生成 %d 隻 %s（半徑 %d 格）...", count, tmpl.Name, radius)

	spawned := 0
	for i := 0; i < count; i++ {
		x := player.X + int32(rand.Intn(int(radius*2+1))) - radius
		y := player.Y + int32(rand.Intn(int(radius*2+1))) - radius

		// 可行走性檢查（最多重試 3 次）
		if deps.MapData != nil {
			ok := deps.MapData.IsPassablePoint(player.MapID, x, y)
			for retry := 0; !ok && retry < 3; retry++ {
				x = player.X + int32(rand.Intn(int(radius*2+1))) - radius
				y = player.Y + int32(rand.Intn(int(radius*2+1))) - radius
				ok = deps.MapData.IsPassablePoint(player.MapID, x, y)
			}
			if !ok {
				continue
			}
		}

		npc := &world.NpcInfo{
			ID:                world.NextNpcID(),
			NpcID:             tmpl.NpcID,
			Impl:              tmpl.Impl,
			GfxID:             tmpl.GfxID,
			Name:              tmpl.Name,
			NameID:            tmpl.NameID,
			Level:             tmpl.Level,
			X:                 x,
			Y:                 y,
			MapID:             player.MapID,
			Heading:           int16(rand.Intn(8)),
			HP:                tmpl.HP,
			MaxHP:             tmpl.HP,
			MP:                tmpl.MP,
			MaxMP:             tmpl.MP,
			AC:                tmpl.AC,
			STR:               tmpl.STR,
			DEX:               tmpl.DEX,
			Intel:             tmpl.INT,
			Exp:               tmpl.Exp,
			Lawful:            tmpl.Lawful,
			Size:              tmpl.Size,
			MR:                tmpl.MR,
			Undead:            tmpl.Undead,
			UndeadType:        tmpl.UndeadType,
			TurnUndeadable:    tmpl.EffectiveTurnUndeadable(),
			TurnUndeadableSet: true,
			Hard:              tmpl.Hard,
			Agro:              tmpl.Agro,
			Family:            tmpl.Family,
			AgroFamily:        tmpl.AgroFamily,
			AtkDmg:            int32(tmpl.Level) + int32(tmpl.STR)/3,
			Ranged:            tmpl.Ranged,
			AtkSpeed:          atkSpeed,
			AtkMagicSpeed:     tmpl.AtkMagicSpeed,
			SubMagicSpeed:     tmpl.SubMagicSpeed,
			MoveSpeed:         moveSpeed,
			PoisonAtk:         tmpl.PoisonAtk,
			FireRes:           tmpl.FireRes,
			WaterRes:          tmpl.WaterRes,
			WindRes:           tmpl.WindRes,
			EarthRes:          tmpl.EarthRes,
			WeakAttr:          tmpl.WeakAttr,
			SpawnX:            x,
			SpawnY:            y,
			SpawnMapID:        player.MapID,
			RespawnDelay:      0, // 壓力測試：不重生
		}
		deps.World.AddNpc(npc)
		spawned++
	}

	gmMsgf(sess, "壓力測試完成：已生成 %d 隻 %s（半徑 %d 格）", spawned, tmpl.Name, radius)
	gmMsg(sess, "走動即可看到怪物，使用 .cleartest 清除")
}

// gmClearTest 清除所有壓力測試用怪物（RespawnDelay == 0 的非永久 NPC）。
func gmClearTest(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	removed := 0
	for _, npc := range deps.World.NpcList() {
		if npc.Dead || npc.RespawnDelay > 0 {
			continue
		}
		npc.HP = 0
		npc.Dead = true
		deps.World.NpcDied(npc)
		npc.DeleteTimer = 1 // 下一 tick 由 NpcRespawnSystem 廣播 RemoveObject
		removed++
	}
	gmMsgf(sess, "已清除 %d 隻測試怪物", removed)
}

// gmClearBuff 清除身上所有 buff（含不可取消的覺醒類）。
// 用法: .clearbuff
func gmClearBuff(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	if deps.Skill == nil {
		gmMsg(sess, "\\f3技能系統未初始化")
		return
	}
	before := 0
	if player.ActiveBuffs != nil {
		before = len(player.ActiveBuffs)
	}
	deps.Skill.GMClearAllStatuses(player)
	gmMsgf(sess, "\\f=已清除 %d 個 buff（含中毒/詛咒/麻痺/隱身）", before)
}

// gmPoison 對自己施加中毒（預設沉默毒/卡司特毒）。
// 用法: .poison              → 沉默毒（卡司特毒）
//
//	.poison damage           → 傷害毒
//	.poison silence          → 沉默毒（同預設）
//	.poison para             → 麻痺毒延遲
func gmPoison(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if deps.GMCmd == nil {
		gmMsg(sess, "\\f3GM 指令系統未初始化")
		return
	}
	// 注意：ptype 對應 npc.PoisonAtk（不是內部 PoisonType）。
	// 1=傷害毒、2=沉默毒、4=麻痺毒延遲，與怪物施毒走完全相同的程式路徑。
	ptype := byte(2) // 預設沉默毒（卡司特毒）
	label := "沉默毒（卡司特毒）"
	if len(args) >= 1 {
		switch strings.ToLower(args[0]) {
		case "damage", "dmg", "1":
			ptype = 1
			label = "傷害毒"
		case "silence", "mute", "2":
			ptype = 2
			label = "沉默毒（卡司特毒）"
		case "para", "paralysis", "4":
			ptype = 4
			label = "麻痺毒"
		default:
			gmMsg(sess, "\\f3用法: .poison [damage|silence|para]")
			return
		}
	}
	if !deps.GMCmd.ApplyPoison(player, ptype) {
		gmMsg(sess, "\\f3已中毒，請先解毒（.clearbuff）")
		return
	}
	gmMsgf(sess, "\\f=已施加 %s", label)
}

// gmWater 切換目前玩家是否在海底地圖看到水（不影響地圖實際 underwater 屬性與其他玩家）。
// 用法: .water           → 切換 on/off
//
//	.water on        → 強制顯示水（依地圖設定）
//	.water off       → 海底地圖也不顯示水
//
// 切換後立即重送 S_MapID 讓客戶端套用新狀態。
func gmWater(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) >= 1 {
		switch strings.ToLower(args[0]) {
		case "on", "1", "true":
			player.WaterOff = false
		case "off", "0", "false":
			player.WaterOff = true
		default:
			gmMsg(sess, "\\f3用法: .water [on|off]")
			return
		}
	} else {
		player.WaterOff = !player.WaterOff
	}
	sendMapIDForPlayer(sess, player, player.MapID, deps)
	if player.WaterOff {
		gmMsg(sess, "\\f=已關閉水的顯示（海底地圖不再顯示水）")
	} else {
		gmMsg(sess, "\\f=已開啟水的顯示（依地圖設定）")
	}
}

// gmBroken 將自己當前裝備的武器耐久損壞值設為指定值（預設 127 極限損壞）。
// 用法: .broken              → Durability = 127
//
//	.broken 30               → Durability = 30
func gmBroken(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if deps.GMCmd == nil {
		gmMsg(sess, "\\f3GM 指令系統未初始化")
		return
	}
	amount := int8(127)
	if len(args) >= 1 {
		v, err := strconv.Atoi(args[0])
		if err != nil || v < 1 || v > 127 {
			gmMsg(sess, "\\f3用法: .broken [數值1-127]")
			return
		}
		amount = int8(v)
	}
	name, ok := deps.GMCmd.BreakWeapon(player, amount)
	if !ok {
		gmMsg(sess, "\\f3未裝備武器")
		return
	}
	gmMsgf(sess, "\\f=已將 %s 損壞值設為 %d", name, amount)
}

// gmSlotTest 發送 S_EquipmentWindow 封包到指定的客戶端裝備欄索引。
// 用於除錯「琮善」客戶端裝備視窗索引映射。
// 用法: .slottest <itemID> <index>       — 將物品顯示在指定欄位
//
//	.slottest scan <itemID>            — 批次掃描 index 0-255，每個間隔 0.5 秒
//	.slottest clear <index>            — 清除指定欄位
//	.slottest clearall                 — 清除 index 0-255 全部
func gmSlotTest(sess *net.Session, player *world.PlayerInfo, args []string) {
	if len(args) < 1 {
		gmMsg(sess, "用法: .slottest <itemID> <index>")
		gmMsg(sess, "      .slottest scan <itemID>  — 批次掃描 0-255")
		gmMsg(sess, "      .slottest clear <index>")
		gmMsg(sess, "      .slottest clearall")
		return
	}

	if args[0] == "clearall" {
		for i := 0; i <= 255; i++ {
			w := packet.NewWriterWithOpcode(packet.S_OPCODE_CHARSYNACK)
			w.WriteC(0x42)
			w.WriteD(int32(0))
			w.WriteC(byte(i))
			w.WriteC(0)
			sess.Send(w.Bytes())
		}
		gmMsg(sess, "已清除 index 0-255 全部欄位")
		return
	}

	if args[0] == "clear" {
		if len(args) < 2 {
			gmMsg(sess, "用法: .slottest clear <index>")
			return
		}
		idx, err := strconv.Atoi(args[1])
		if err != nil {
			gmMsg(sess, "\\f3無效的 index")
			return
		}
		w := packet.NewWriterWithOpcode(packet.S_OPCODE_CHARSYNACK)
		w.WriteC(0x42)
		w.WriteD(int32(0))
		w.WriteC(byte(idx))
		w.WriteC(0)
		sess.Send(w.Bytes())
		gmMsgf(sess, "已清除欄位 index=%d", idx)
		return
	}

	if args[0] == "scan" {
		if len(args) < 2 {
			gmMsg(sess, "用法: .slottest scan <itemID>")
			return
		}
		id, err := strconv.Atoi(args[1])
		if err != nil {
			gmMsg(sess, "\\f3無效的 itemID")
			return
		}
		item := player.Inv.FindByObjectID(int32(id))
		if item == nil {
			item = player.Inv.FindByItemID(int32(id))
		}
		if item == nil {
			gmMsgf(sess, "\\f3背包中找不到物品 ID=%d", id)
			return
		}
		gmMsgf(sess, "開始掃描 %s (objID=%d) index 0-255...", item.Name, item.ObjectID)
		for i := 0; i <= 255; i++ {
			w := packet.NewWriterWithOpcode(packet.S_OPCODE_CHARSYNACK)
			w.WriteC(0x42)
			w.WriteD(item.ObjectID)
			w.WriteC(byte(i))
			w.WriteC(1)
			sess.Send(w.Bytes())
		}
		gmMsg(sess, "掃描完成(0-255)。檢查裝備視窗，用 .slottest clearall 清除。")
		return
	}

	if len(args) < 2 {
		gmMsg(sess, "用法: .slottest <itemID> <index>")
		return
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		gmMsg(sess, "\\f3無效的 itemID")
		return
	}
	idx, err := strconv.Atoi(args[1])
	if err != nil {
		gmMsg(sess, "\\f3無效的 index")
		return
	}

	item := player.Inv.FindByObjectID(int32(id))
	if item == nil {
		item = player.Inv.FindByItemID(int32(id))
	}
	if item == nil {
		gmMsgf(sess, "\\f3背包中找不到物品 ID=%d", id)
		return
	}

	w := packet.NewWriterWithOpcode(packet.S_OPCODE_CHARSYNACK)
	w.WriteC(0x42)
	w.WriteD(item.ObjectID)
	w.WriteC(byte(idx))
	w.WriteC(1)
	sess.Send(w.Bytes())
	gmMsgf(sess, "已發送 %s (objID=%d) 到欄位 index=%d", item.Name, item.ObjectID, idx)
}

// gmSlotExpand 發送 S_CharReset(67) 擴充欄位封包，用於測試不同 subType/value。
// 用法: .slotexpand <subType> <value>
// 已知: subType=1 value=7→Ring3, value=15→Ring4（正常）
//
//	subType=2 value=1~3→符文（琮善客戶端可能異常）
func gmSlotExpand(sess *net.Session, args []string) {
	if len(args) < 2 {
		gmMsg(sess, "用法: .slotexpand <subType> <value>")
		gmMsg(sess, "  已知: 1,7=Ring3 | 1,15=Ring4 | 2,1~3=符文(可能異常)")
		return
	}
	subType, err := strconv.Atoi(args[0])
	if err != nil {
		gmMsg(sess, "\\f3無效的 subType")
		return
	}
	value, err := strconv.Atoi(args[1])
	if err != nil {
		gmMsg(sess, "\\f3無效的 value")
		return
	}

	w := packet.NewWriterWithOpcode(packet.S_OPCODE_CHARSYNACK)
	w.WriteC(67) // sub-type: 擴充欄位
	w.WriteD(int32(subType))
	w.WriteC(byte(value))
	for i := 0; i < 6; i++ {
		w.WriteD(0)
	}
	w.WriteH(0)
	sess.Send(w.Bytes())
	gmMsgf(sess, "已發送 S_CharReset(67, %d, %d)", subType, value)
}

// gmSlotExpand2 使用 S_EquipmentWindow 第二建構函式的格式發送擴充封包。
// 與 gmSlotExpand 的差異：value 用 writeD (4B) 而非 writeC (1B)，padding 也不同。
// 用法: .slotexpand2 <type> <value>
func gmSlotExpand2(sess *net.Session, args []string) {
	if len(args) < 2 {
		gmMsg(sess, "用法: .slotexpand2 <type> <value>")
		gmMsg(sess, "  S_EquipmentWindow 格式: [67][D:type][D:value][D:0]*3[C:0]")
		return
	}
	t, err := strconv.Atoi(args[0])
	if err != nil {
		gmMsg(sess, "\\f3無效的 type")
		return
	}
	v, err := strconv.Atoi(args[1])
	if err != nil {
		gmMsg(sess, "\\f3無效的 value")
		return
	}

	// 琮善 S_EquipmentWindow 第二建構函式格式
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_CHARSYNACK)
	w.WriteC(0x43)     // sub-type 67 (0x43)
	w.WriteD(int32(t)) // type: 2=符文
	w.WriteD(int32(v)) // value: 3=三個欄位（writeD, 非 writeC）
	w.WriteD(0)        // padding
	w.WriteD(0)        // padding
	w.WriteD(0)        // padding
	w.WriteC(0)        // padding
	sess.Send(w.Bytes())
	gmMsgf(sess, "已發送 S_EquipmentWindow(0x43, %d, %d) — writeD 格式", t, v)
}

// gmInvisible follows Java L1Invisible: repeated commands remain invisible.
func gmInvisible(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	gmSetInvisible(sess, player, true, deps)
}

func gmSetInvisible(sess *net.Session, player *world.PlayerInfo, invisible bool, deps *Deps) {
	player.GMInvisible = invisible
	SendInvisible(sess, player.CharID, player.IsInvisible())

	ws := deps.World
	nearby := ws.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, sess.ID, player.ShowID)

	if invisible {
		// 隱身：周圍玩家移除我的角色顯示
		for _, other := range nearby {
			SendRemoveObject(other.Session, player.CharID)
			if other.Known != nil {
				delete(other.Known.Players, player.CharID)
			}
		}
		gmMsg(sess, "\\f2GM 隱身已開啟。")
	} else {
		// 解除隱身：周圍玩家重新顯示我
		for _, other := range nearby {
			if other.CanSeePlayer(player) {
				SendPutObject(other.Session, player)
				if other.Known == nil {
					other.Known = world.NewKnownEntities()
				}
				other.Known.Players[player.CharID] = world.KnownPos{X: player.X, Y: player.Y}
			}
		}
		gmMsg(sess, "\\f2GM 隱身已關閉。")
	}
}
