package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
)

func dispatchGMCharacter(sess *net.Session, player *world.PlayerInfo, cmd string, args []string, deps *Deps) bool {
	switch cmd {
	case "setting":
		gmSetting(sess, player, args, deps)
	case "desc":
		target := gmCharacterTarget(sess, player, args, deps)
		if target != nil {
			gmShowInfo(sess, target)
			gmMsgf(sess, "善惡值:%d 權限:%d GM:%t 監察員:%t", target.Karma, target.AccessLevel, target.IsGM(), target.IsMonitor())
		}
	case "death":
		if len(args) != 1 {
			gmMsg(sess, "用法：.death <玩家>")
			return true
		}
		target := gmCharacterTarget(sess, player, args, deps)
		if target != nil && deps.Death != nil {
			target.HP = 0
			deps.Death.KillPlayer(target)
			gmMsgf(sess, "已執行死亡處理：%s", target.Name)
		}
	case "ress":
		gmRestoreNearby(sess, player, deps)
	case "buff":
		gmBuffJava(sess, player, args, deps)
	case "allbuff":
		target := gmCharacterTarget(sess, player, args, deps)
		if target == nil || deps.Skill == nil {
			return true
		}
		count := 0
		for _, id := range []int32{2, 14, 26, 32, 42, 48, 55, 68, 79, 88, 89, 90, 98, 102, 104, 105, 106, 111, 114, 117, 129, 137, 147, 160, 163, 168, 169, 170, 171, 175, 176} {
			if gmApplyBuffDuration(target, id, 9999, deps) {
				count++
			}
		}
		gmSetHasteBrave(target, 9999, false, deps)
		if deps.Polymorph != nil {
			deps.Polymorph.DoPoly(target, 5641, 9999, data.PolyCauseGM)
		}
		gmMsgf(sess, "已替 %s 套用 %d 個輔助魔法。", target.Name, count)
	case "clearallbuff":
		target := gmCharacterTarget(sess, player, args, deps)
		if target != nil {
			gmClearBuff(sess, target, deps)
		}
	case "speed":
		if len(args) > 0 {
			return false
		}
		gmSetHasteBrave(player, 3600, true, deps)
		gmMsg(sess, "已套用加速、勇敢與三段加速。")
	default:
		return false
	}
	return true
}

func gmCharacterTarget(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) *world.PlayerInfo {
	if len(args) == 0 || strings.EqualFold(args[0], "me") {
		return player
	}
	if deps != nil && deps.World != nil {
		if target := deps.World.GetByName(args[0]); target != nil {
			return target
		}
	}
	gmMsgf(sess, "找不到線上玩家：%s", args[0])
	return nil
}

func gmSetting(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) != 3 {
		gmMsg(sess, "用法：.setting <玩家|me> <屬性|gm> <值>")
		return
	}
	value, err := strconv.ParseInt(args[2], 10, 32)
	if err != nil {
		gmMsg(sess, "數值必須是 32 位元整數。")
		return
	}
	target := gmCharacterTarget(sess, player, args[:1], deps)
	if target == nil {
		return
	}
	param := strings.ToLower(args[1])
	if param == "gm" {
		if value != 0 && value != 100 && value != 200 {
			gmMsg(sess, "身分權限：0=玩家、100=監察員、200=GM。")
			return
		}
		if deps.CharRepo == nil {
			gmMsg(sess, "角色資料庫未初始化。")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := deps.CharRepo.SetAccessLevel(ctx, target.Name, int16(value)); err != nil {
			gmMsgf(sess, "權限儲存失敗：%v", err)
			return
		}
		target.AccessLevel = int16(value)
		target.SetGM(value == 200)
		if value != 200 {
			target.GMFindInvis, target.GMShowTraps = false, false
		}
		if !target.CanUseGMCommands() {
			gmSetInvisible(target.Session, target, false, deps)
			target.GMHPBar, target.GMShowTraps = false, false
			UpdateGMWorldView(target, deps)
		}
		gmMsgf(sess, "%s 的權限已設為 %d，立即生效。", target.Name, value)
		return
	}
	if deps.GMCmd == nil {
		gmMsg(sess, "GM 系統未初始化。")
		return
	}
	base := gmCharacterRow(target)
	persistValue := true
	switch param {
	case "str", "dex", "con", "wis", "int", "cha":
		if value < 1 || value > 127 {
			gmMsg(sess, "屬性必須介於 1-127。")
			return
		}
		bonus := int16(0)
		switch param {
		case "str":
			bonus = target.Str - base.Str
		case "dex":
			bonus = target.Dex - base.Dex
		case "con":
			bonus = target.Con - base.Con
		case "wis":
			bonus = target.Wis - base.Wis
		case "int":
			bonus = target.Intel - base.Intel
		case "cha":
			bonus = target.Cha - base.Cha
		}
		deps.GMCmd.SetStat(target.Session, target, param, int16(value)+bonus)
	case "hp", "mp":
		if value < 1 || value > 32767 {
			gmMsg(sess, "HP/MP 上限必須介於 1-32767。")
			return
		}
		if param == "hp" {
			target.MaxHP = int32(value) + target.MaxHP - base.MaxHP
			deps.GMCmd.SetHP(target.Session, target, int(target.MaxHP))
		} else {
			target.MaxMP = int32(value) + target.MaxMP - base.MaxMP
			deps.GMCmd.SetMP(target.Session, target, int(target.MaxMP))
		}
	case "lawful":
		if value < -32768 || value > 32767 {
			gmMsg(sess, "正義值必須介於 -32768 至 32767。")
			return
		}
		deps.GMCmd.AdjustLawful(target.Session, target, int32(value)-target.Lawful)
	case "karma":
		target.Karma = int32(value)
	case "ac", "mr", "hit", "dmg":
		if value < -32768 || value > 32767 {
			gmMsg(sess, "戰鬥屬性必須介於 -32768 至 32767。")
			return
		}
		persistValue = false
		switch param {
		case "ac":
			target.AC = int16(value)
		case "mr":
			target.MR = int16(value)
		case "hit":
			target.HitMod = int16(value)
		case "dmg":
			target.DmgMod = int16(value)
		}
	default:
		gmMsg(sess, "未知屬性；輸入 .help setting 查看用法。")
		return
	}
	target.Dirty = true
	SendPlayerStatus(target.Session, target)
	if persistValue && deps.CharRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := deps.CharRepo.SaveCharacter(ctx, gmCharacterRow(target)); err != nil {
			gmMsgf(sess, "數值已更新，儲存失敗：%v", err)
			return
		}
	}
	gmMsgf(sess, "%s 的 %s 已設為 %d。", target.Name, param, value)
}

// Preserve every persisted field when a GM changes a single attribute.
func gmCharacterRow(p *world.PlayerInfo) *persist.CharacterRow {
	row := &persist.CharacterRow{Name: p.Name, Level: p.Level, Exp: int64(p.Exp), HP: p.HP, MP: p.MP,
		OriginalStr: p.OriginalStr, OriginalCon: p.OriginalCon,
		MaxHP: p.MaxHP, MaxMP: p.MaxMP, X: p.X, Y: p.Y, MapID: p.MapID, Heading: p.Heading,
		Lawful: p.Lawful, Str: p.Str, Dex: p.Dex, Con: p.Con, Wis: p.Wis, Cha: p.Cha, Intel: p.Intel,
		BonusStats: p.BonusStats, ElixirStats: p.ElixirStats, ClanID: p.ClanID, ClanName: p.ClanName,
		ClanRank: p.ClanRank, Title: p.Title, Karma: p.Karma, PKCount: p.PKCount, Food: p.Food}
	eq := p.EquipBonuses
	row.Str -= int16(eq.AddStr)
	row.Dex -= int16(eq.AddDex)
	row.Con -= int16(eq.AddCon)
	row.Wis -= int16(eq.AddWis)
	row.Intel -= int16(eq.AddInt)
	row.Cha -= int16(eq.AddCha)
	row.MaxHP -= int32(eq.AddHP)
	row.MaxMP -= int32(eq.AddMP)
	for _, buff := range p.ActiveBuffs {
		row.Str -= buff.DeltaStr
		row.Dex -= buff.DeltaDex
		row.Con -= buff.DeltaCon
		row.Wis -= buff.DeltaWis
		row.Intel -= buff.DeltaIntel
		row.Cha -= buff.DeltaCha
		row.MaxHP -= buff.DeltaMaxHP
		row.MaxMP -= buff.DeltaMaxMP
	}
	return row
}

func gmApplyBuffDuration(target *world.PlayerInfo, id, seconds int32, deps *Deps) bool {
	if applier, ok := deps.Skill.(interface {
		ApplyGMBuffWithDuration(*world.PlayerInfo, int32, int32) bool
	}); ok {
		return applier.ApplyGMBuffWithDuration(target, id, seconds)
	}
	return deps.Skill.ApplyGMBuff(target, id)
}

func gmBuffJava(sess *net.Session, player *world.PlayerInfo, args []string, deps *Deps) {
	if len(args) == 0 || deps.Skill == nil || deps.Skills == nil {
		gmMsg(sess, "用法：.buff [me|all] <技能 ID> [秒數]")
		return
	}
	mode := "nearby"
	if args[0] == "me" || args[0] == "all" {
		mode, args = args[0], args[1:]
	}
	if len(args) < 1 || len(args) > 2 {
		gmMsg(sess, "用法：.buff [me|all] <技能 ID> [秒數]")
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil || id < 1 {
		gmMsg(sess, "無效的技能 ID。")
		return
	}
	skill := deps.Skills.Get(int32(id))
	if skill == nil || (skill.Target != "buff" && skill.Target != "none") {
		gmMsg(sess, "只能使用 buff/none 類型魔法。")
		return
	}
	seconds := int64(0)
	if len(args) == 2 {
		seconds, err = strconv.ParseInt(args[1], 10, 32)
		if err != nil || seconds < 0 || seconds > 65535 {
			gmMsg(sess, "秒數必須介於 0-65535。")
			return
		}
	}
	count := 0
	apply := func(p *world.PlayerInfo) {
		if !p.Dead && gmApplyBuffDuration(p, int32(id), int32(seconds), deps) {
			count++
		}
	}
	switch mode {
	case "me":
		apply(player)
	case "all":
		deps.World.AllPlayers(apply)
	default:
		for _, p := range deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, player.SessionID, player.ShowID) {
			apply(p)
		}
	}
	gmMsgf(sess, "已對 %d 位玩家套用 %s。", count, skill.Name)
}

func gmSetHasteBrave(player *world.PlayerInfo, seconds int, third bool, deps *Deps) {
	player.MoveSpeed, player.BraveSpeed = 1, 1
	player.HasteTicks, player.BraveTicks = seconds*5, seconds*5
	if third {
		player.AddBuff(&world.ActiveBuff{SkillID: SkillStatusThirdSpeed, TicksLeft: 600 * 5})
	}
	for _, viewer := range deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID) {
		if !viewer.CanSeePlayer(player) {
			continue
		}
		sendSpeedPacket(viewer.Session, player.CharID, 1, uint16(seconds))
		sendBravePacket(viewer.Session, player.CharID, 1, uint16(seconds))
		if third {
			viewer.Session.Send(BuildLiquor(player.CharID, 8))
			viewer.Session.Send(BuildSkillEffect(player.CharID, 7976))
		}
	}
	if third {
		SendServerMessage(player.Session, 1065)
	}
	SendPlayerStatus(player.Session, player)
}

func gmRestoreNearby(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	if deps.GMCmd == nil {
		gmMsg(sess, "GM 系統未初始化。")
		return
	}
	deps.GMCmd.FullHeal(sess, player)
	BroadcastToPlayers(deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID), BuildSkillEffect(player.CharID, 759))
	for _, target := range deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, player.SessionID, player.ShowID) {
		gfx := int32(832)
		if target.Dead {
			gfx = 3944
			target.PendingResCaster, target.PendingResSkill = player.CharID, 75
			SendYesNoDialog(target.Session, 322)
		} else {
			deps.GMCmd.FullHeal(target.Session, target)
		}
		BroadcastToPlayers(deps.World.GetNearbyPlayersInShow(target.X, target.Y, target.MapID, 0, target.ShowID), BuildSkillEffect(target.CharID, gfx))
	}
	gmMsg(sess, "已治療附近玩家，並向死亡玩家送出復活確認。")
}
