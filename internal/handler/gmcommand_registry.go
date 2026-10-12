package handler

import (
	"sort"
	"strconv"
	"strings"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// GMCommandSpec keeps aliases, access checks and help in one place. Levels and
// Chinese aliases follow L1JTW-Eddy db/l1jtw_InnoDB.sql's commands table.
type GMCommandSpec struct {
	Name    string
	Level   int16
	Aliases string
	Usage   string
}

var gmCommands = []GMCommandSpec{
	{"echo", 200, "公告", "<文字>（回顯本人）"},
	{"setting", 200, "設定", "<玩家|me> <hp|mp|str|dex|con|wis|int|cha|ac|mr|hit|dmg|lawful|karma|gm> <值>"},
	{"summon", 150, "召寵", "<NPC ID|名稱> [數量]"},
	{"cleaning", 200, "清除", ""},
	{"addskill", 200, "技能 allskill", ""},
	{"level", 150, "等級", "<1-99>"},
	{"loc", 50, "座標 pos coord", "[玩家]"},
	{"desc", 200, "詳情", "[玩家|me]"},
	{"who", 100, "玩家", ""},
	{"allbuff", 200, "全輔助", "[玩家|me]"},
	{"clearallbuff", 200, "清除輔助 clearbuff", "[玩家|me]"},
	{"speed", 50, "加速", "[0-4]"},
	{"adena", 150, "金幣 gold", "<數量>"},
	{"hpbar", 50, "血條", "<on|off>"},
	{"resettrap", 200, "重設陷阱", ""},
	{"reloadtrap", 200, "重讀陷阱", ""},
	{"showtrap", 200, "顯示陷阱", "<on|off>"},
	{"castgfx", 200, "施法動畫", "<動畫 ID>"},
	{"gfxid", 200, "影像動畫", "<起始 ID> <數量>"},
	{"invgfxid", 200, "背包影像", "<起始 ID> <數量>"},
	{"hometown", 150, "村莊", "<daily|monthly>"},
	{"gm", 150, "遊戲管理員", "[on|off]"},
	{"present", 200, "贈送", "<帳號|*> <物品 ID> <強化> <數量>"},
	{"lvpresent", 200, "限等贈送", "<最低等級> <最高等級> <物品 ID> <強化> <數量>"},
	{"shutdown", 200, "關機", "<秒數|now|abort>"},
	{"item", 150, "物品", "<物品 ID|名稱> [數量] [強化]"},
	{"itemset", 150, "套裝", "<套裝名稱>"},
	{"buff", 100, "輔助", "[me|all] <技能 ID> [秒數]"},
	{"patrol", 50, "查詢", ""},
	{"banip", 150, "封鎖IP", "<IP> [add|del]"},
	{"chat", 50, "廣播頻道", "<on|off>"},
	{"chatng", 50, "禁言", "<玩家> <分鐘，0=解除>"},
	{"skick", 150, "skick1", "<玩家>"},
	{"kick", 50, "kick1", "<玩家>"},
	{"powerkick", 150, "powerkick1", "<玩家>"},
	{"accbankick", 150, "封鎖帳號", "<玩家>"},
	{"poly", 150, "變身", "<玩家> <變身 ID> 或 <變身 ID> [玩家]"},
	{"ress", 100, "恢復", ""},
	{"death", 200, "死亡", "<玩家>"},
	{"gmroom", 50, "房間", "<1-5|房間名稱>"},
	{"topc", 50, "前往玩家 goto", "<玩家>"},
	{"move", 50, "移動 warp teleport", "<x> <y> [地圖]"},
	{"weather", 150, "天氣", "<天氣代碼>"},
	{"tospawn", 200, "前往怪物", "[生成點 ID|+|-]"},
	{"f", 50, "書籤", "<set 指令 [參數]|show|代入參數>"},
	{"recall", 100, "召回", "<玩家>"},
	{"visible", 50, "解除隱身", ""},
	{"partyrecall", 100, "召團", "<玩家>"},
	{"invisible", 50, "隱身", ""},
	{"spawn", 100, "召怪", "<NPC ID|名稱> [數量] [範圍]"},
	{"insert", 200, "創怪", "<mob|npc> <NPC ID>"},
	{"help", 50, "說明", "[頁碼|指令]"},
	{"action", 200, "動作", "<動作 ID>"},
	{"tile", 200, "tile1", ""},
	{"findinvis", 200, "findinvis1", "<on|off>"},
	{"save", 200, "儲存", ""},
	{"system", 200, "系統", ""},
	// Java later migrations add these administration commands.
	{"marquee", 200, "", "<訊息內容>"},
	{"cpadd", 200, "", "<帳號> [幣別] <正負點數> <備註>"},
	{"cpquery", 200, "", "<帳號> [幣別]"},
	{"cpshopreload", 200, "", ""},
	{"advpolygive", 200, "", "<玩家> <變身 ID|卡片 ID>"},
	{"advpolyunlock", 200, "", "<玩家> <變身 ID>"},
	{"advpolyreset", 200, "", "<玩家>"},
	// Go development commands are retained, with explicit administrator access.
	{"hp", 200, "", "<HP>"}, {"mp", 200, "", "<MP>"},
	{"heal", 200, "", ""}, {"stat", 200, "", "<屬性> <值>"},
	{"lawful", 200, "law l", "<正負增量> [玩家]"},
	{"spell", 200, "", "<技能 ID|0>"},
	{"kill", 200, "", ""}, {"killall", 200, "", ""},
	{"exp", 200, "", "<經驗值>"}, {"class", 200, "", "<0-6>"},
	{"rez", 200, "resurrect", "[玩家]"}, {"ac", 200, "", ""},
	{"polygfx", 200, "", "<影像 ID>"}, {"undopoly", 150, "", "[玩家]"},
	{"wall", 200, "", "[1|2|3]"}, {"clearwall", 200, "", ""},
	{"poison", 200, "", "[damage|silence|para]"}, {"broken", 200, "", "[1-127]"},
	{"water", 200, "", "[on|off]"}, {"stresstest", 200, "", "<NPC ID> [數量] [半徑]"},
	{"cleartest", 200, "", ""}, {"slottest", 200, "", "<參數>"},
	{"slotexpand", 200, "", "<type> <value>"}, {"slotexpand2", 200, "", "<type> <value>"},
	{"time", 200, "", "<時間>"}, {"r", 50, "", "[替換參數]"},
}

var gmCommandIndex = func() map[string]*GMCommandSpec {
	index := make(map[string]*GMCommandSpec)
	for i := range gmCommands {
		cmd := &gmCommands[i]
		for _, name := range append([]string{cmd.Name}, strings.Fields(cmd.Aliases)...) {
			name = strings.ToLower(name)
			if index[name] != nil {
				panic("duplicate GM command: " + name)
			}
			index[name] = cmd
		}
	}
	return index
}()

func gmCommandHelp(sess *net.Session, player *world.PlayerInfo, args []string) {
	page := 1
	if len(args) > 0 {
		if cmd := gmCommandIndex[strings.ToLower(strings.TrimPrefix(args[0], "."))]; cmd != nil {
			if player.AccessLevel < cmd.Level {
				gmMsg(sess, "你的權限不足以使用此指令。")
				return
			}
			gmMsgf(sess, ".%s %s (權限 %d)", cmd.Name, cmd.Usage, cmd.Level)
			if cmd.Aliases != "" {
				gmMsgf(sess, "別名：%s", cmd.Aliases)
			}
			return
		}
		var err error
		page, err = strconv.Atoi(args[0])
		if err != nil {
			gmMsg(sess, "用法：.help [頁碼|指令]")
			return
		}
	}
	var available []GMCommandSpec
	for _, cmd := range gmCommands {
		if player.AccessLevel >= cmd.Level {
			available = append(available, cmd)
		}
	}
	sort.Slice(available, func(i, j int) bool { return available[i].Name < available[j].Name })
	const pageSize = 12
	pages := (len(available) + pageSize - 1) / pageSize
	if page < 1 || page > pages {
		gmMsgf(sess, "頁碼必須在 1-%d 之間。", pages)
		return
	}
	gmMsgf(sess, "GM 指令 %d/%d（.help 指令 可查中文別名）", page, pages)
	for i := (page - 1) * pageSize; i < len(available) && i < page*pageSize; i++ {
		cmd := available[i]
		gmMsgf(sess, ".%s %s", cmd.Name, cmd.Usage)
	}
}
