package handler

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// GMAdminState 的資料只由遊戲迴圈存取。關機回呼將工作交回主程式的正常存檔退出路徑。
type GMAdminState struct {
	WorldChatDisabled bool
	ShutdownAt        time.Time
	SaveAll           func() error
	Shutdown          func()
	lastCountdown     int
}

func NewGMAdminState() *GMAdminState { return &GMAdminState{} }

const SkillStatusChatProhibited int32 = 1005

func GMWorldChatEnabled(deps *Deps) bool {
	return deps == nil || deps.GMAdmin == nil || !deps.GMAdmin.WorldChatDisabled
}

// IsGMChatProhibited 使用既有 buff 持久化及 tick 到期機制，重新登入不能逃避禁言。
func IsGMChatProhibited(player *world.PlayerInfo, _ *Deps) bool {
	return player != nil && player.HasBuff(SkillStatusChatProhibited)
}

// GMAdminTick 必須從遊戲迴圈呼叫，避免倒數計時器 goroutine 與世界狀態競爭。
func GMAdminTick(now time.Time, deps *Deps) {
	if deps == nil || deps.GMAdmin == nil || deps.GMAdmin.ShutdownAt.IsZero() {
		return
	}
	a := deps.GMAdmin
	remaining := int((a.ShutdownAt.Sub(now) + time.Second - 1) / time.Second)
	if remaining <= 0 {
		a.ShutdownAt = time.Time{}
		if a.Shutdown != nil {
			a.Shutdown()
		}
		return
	}
	if remaining != a.lastCountdown && (remaining <= 10 || remaining%60 == 0) {
		gmBroadcast(deps, fmt.Sprintf("伺服器將於 %d 秒後關閉，請安全登出。", remaining))
	}
	a.lastCountdown = remaining
}

func dispatchGMAdmin(sess *net.Session, player *world.PlayerInfo, cmd string, args []string, deps *Deps) bool {
	switch cmd {
	case "echo":
		gmMsg(sess, strings.Join(args, " "))
	case "marquee":
		gmAdminMarquee(sess, args, deps)
	case "advpolygive", "advpolyunlock", "advpolyreset":
		gmAdminAdvPoly(sess, cmd, args, deps)
	case "chat":
		gmAdminChat(sess, args, deps)
	case "chatng":
		gmAdminChatNG(sess, args, deps)
	case "kick", "skick", "powerkick", "accbankick":
		gmAdminKick(sess, cmd, args, deps)
	case "banip":
		gmAdminBanIP(sess, args, deps)
	case "shutdown":
		gmAdminShutdown(sess, args, deps)
	case "system":
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		gmMsgf(sess, "系統: %s/%s %s；CPU:%d goroutine:%d；使用記憶體:%d MiB；線上:%d", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU(), runtime.NumGoroutine(), m.Alloc/1024/1024, deps.World.PlayerCount())
	case "save":
		gmAdminSave(sess, deps)
	case "present", "lvpresent":
		gmAdminPresent(sess, cmd, args, deps)
	case "cleaning":
		gmAdminCleaning(sess, deps)
	case "hometown":
		gmAdminHomeTown(sess, args, deps)
	default:
		return false
	}
	return true
}

func gmAdminState(deps *Deps) *GMAdminState {
	if deps.GMAdmin == nil {
		deps.GMAdmin = NewGMAdminState()
	}
	return deps.GMAdmin
}

func gmBroadcast(deps *Deps, message string) {
	if deps.World == nil {
		return
	}
	deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if p.Session != nil {
			gmMsg(p.Session, message)
		}
	})
}

// gmAdminMarquee 對齊 Java L1Marquee：全服 S_RedMessage(3440,message)，再回覆發送者。
// 客戶端 string-c.tbl 的訊息 3440（原 dummy3442）需設為 %0。
func gmAdminMarquee(sess *net.Session, args []string, deps *Deps) {
	message := strings.TrimSpace(strings.Join(args, " "))
	if message == "" {
		gmMsg(sess, ".marquee 訊息內容（支援 \\f 色碼，如 \\f3紅色 \\f>白色）")
		return
	}
	if deps == nil || deps.World == nil {
		gmMsg(sess, "世界狀態尚未初始化，無法廣播跑馬燈。")
		return
	}
	deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if p.Session != nil {
			SendRedMessage(p.Session, 3440, message)
		}
	})
	gmMsg(sess, "跑馬燈廣播已發送："+message)
}

func gmAdminChat(sess *net.Session, args []string, deps *Deps) {
	a := gmAdminState(deps)
	if len(args) == 0 {
		if a.WorldChatDisabled {
			gmMsg(sess, "全體聊天已關閉。.chat on 能使其開啟。")
		} else {
			gmMsg(sess, "全體聊天已開啟。.chat off 能使其關閉。")
		}
		return
	}
	if len(args) != 1 {
		gmMsg(sess, "用法: .chat [on|off]")
		return
	}
	switch strings.ToLower(args[0]) {
	case "on":
		a.WorldChatDisabled = false
		gmMsg(sess, "開啟全體聊天。")
	case "off":
		a.WorldChatDisabled = true
		gmMsg(sess, "關閉全體聊天。")
	default:
		gmMsg(sess, "用法: .chat [on|off]")
	}
}

func gmAdminChatNG(sess *net.Session, args []string, deps *Deps) {
	if len(args) != 2 {
		gmMsg(sess, "用法: .chatng 玩家名稱 時間(分，0=解除)")
		return
	}
	minutes, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil || minutes < 0 || minutes > 35791394 {
		gmMsg(sess, "禁言時間必須為非負整數且不得超過 35791394 分鐘。")
		return
	}
	target := gmAdminTarget(sess, args[0], deps)
	if target == nil {
		return
	}
	if minutes == 0 {
		target.RemoveBuff(SkillStatusChatProhibited)
		sendIconGfx(target.Session, 36, 0)
		gmMsg(target.Session, "GM 已解除您的禁言。")
	} else {
		target.AddBuff(&world.ActiveBuff{SkillID: SkillStatusChatProhibited, TicksLeft: int(minutes) * 60 * 5})
		seconds := minutes * 60
		if seconds > 65535 {
			seconds = 65535
		}
		sendIconGfx(target.Session, 36, uint16(seconds))
		sendServerMessageArgs(target.Session, 286, args[1])
	}
	target.Dirty = true
	gmMsgf(sess, "%s 禁言時間已設為 %d 分鐘。", target.Name, minutes)
}

func gmAdminTarget(sess *net.Session, name string, deps *Deps) *world.PlayerInfo {
	var target *world.PlayerInfo
	if deps.World != nil {
		target = deps.World.GetByName(name)
		if target == nil {
			deps.World.AllPlayers(func(p *world.PlayerInfo) {
				if strings.EqualFold(p.Name, name) {
					target = p
				}
			})
		}
	}
	if target == nil || target.Session == nil {
		gmMsgf(sess, "找不到線上角色: %s", name)
		return nil
	}
	return target
}

func gmAdminKick(sess *net.Session, cmd string, args []string, deps *Deps) {
	if len(args) != 1 {
		gmMsgf(sess, "用法: .%s 玩家名稱", cmd)
		return
	}
	target := gmAdminTarget(sess, args[0], deps)
	if target == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cmd == "powerkick" || cmd == "accbankick" {
		if deps.AccountRepo == nil {
			gmMsg(sess, "帳號資料庫尚未初始化。")
			return
		}
		var err error
		if cmd == "powerkick" {
			err = deps.AccountRepo.SetIPBanned(ctx, target.Session.IP, true, sess.AccountName)
		} else {
			err = deps.AccountRepo.SetBanned(ctx, target.Session.AccountName, true)
		}
		if err != nil {
			gmAdminError(sess, deps, "封鎖失敗", err)
			return
		}
	}
	if cmd == "skick" {
		// Java SKick：下次登入重定位到銀騎士村，仍交給 InputSystem 完成正常登出存檔。
		if deps.MapData != nil {
			deps.MapData.SetImpassable(target.MapID, target.X, target.Y, false)
		}
		deps.World.UpdatePosition(target.SessionID, 33080, 33392, 4, target.Heading)
		target.ShowID = 0
		target.Dirty = true
	}
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_KICK)
	w.WriteH(500)
	w.WriteD(0)
	target.Session.Send(w.Bytes())
	target.Session.FlushOutput()
	// 僅要求斷線狀態；下一輪 InputSystem 執行既有清理和存檔，不能刪除玩家繞過存檔。
	target.Session.Close()
	gmMsgf(sess, "%s 已被踢除遊戲 (%s)。", target.Name, cmd)
}

func gmAdminBanIP(sess *net.Session, args []string, deps *Deps) {
	if len(args) < 1 || len(args) > 2 {
		gmMsg(sess, "用法: .banip IP [add|del]")
		return
	}
	ip, err := persist.NormalizeIPAddress(args[0])
	if err != nil {
		gmMsg(sess, "IP 位址格式錯誤。")
		return
	}
	if len(args) == 2 && !strings.EqualFold(args[1], "add") && !strings.EqualFold(args[1], "del") {
		gmMsg(sess, "用法: .banip IP [add|del]")
		return
	}
	if deps.AccountRepo == nil {
		gmMsg(sess, "帳號資料庫尚未初始化。")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if len(args) == 2 {
		err = deps.AccountRepo.SetIPBanned(ctx, ip, strings.EqualFold(args[1], "add"), sess.AccountName)
		if err != nil {
			gmAdminError(sess, deps, "IP 封鎖更新失敗", err)
			return
		}
	}
	banned, err := deps.AccountRepo.IsIPBanned(ctx, ip)
	if err != nil {
		gmAdminError(sess, deps, "IP 封鎖查詢失敗", err)
		return
	}
	deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if p.Session == nil {
			return
		}
		otherIP, _ := persist.NormalizeIPAddress(p.Session.IP)
		if otherIP == ip {
			gmMsgf(sess, "IP:%s 連線中的角色名稱:%s", ip, p.Name)
		}
	})
	if banned {
		gmMsgf(sess, "IP:%s 已被登記在封鎖名單中。", ip)
	} else {
		gmMsgf(sess, "IP:%s 尚未被登記在封鎖名單中。", ip)
	}
}

func gmAdminShutdown(sess *net.Session, args []string, deps *Deps) {
	if len(args) != 1 {
		gmMsg(sess, "用法: .shutdown 秒數|now|abort")
		return
	}
	a := gmAdminState(deps)
	if strings.EqualFold(args[0], "abort") {
		a.ShutdownAt = time.Time{}
		gmBroadcast(deps, "伺服器關機倒數已取消。")
		return
	}
	if a.Shutdown == nil {
		gmMsg(sess, "關機服務尚未初始化。")
		return
	}
	if strings.EqualFold(args[0], "now") {
		a.ShutdownAt = time.Time{}
		gmBroadcast(deps, "伺服器正在存檔並關閉。")
		a.Shutdown()
		return
	}
	seconds, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		gmMsg(sess, "用法: .shutdown 秒數|now|abort")
		return
	}
	if seconds < 5 {
		seconds = 5
	}
	a.ShutdownAt = time.Now().Add(time.Duration(seconds) * time.Second)
	a.lastCountdown = int(seconds)
	gmBroadcast(deps, fmt.Sprintf("伺服器將於 %d 秒後關閉，請安全登出。", seconds))
}

func gmAdminSave(sess *net.Session, deps *Deps) {
	a := gmAdminState(deps)
	if a.SaveAll == nil {
		gmMsg(sess, "存檔服務尚未初始化。")
		return
	}
	if err := a.SaveAll(); err != nil {
		gmAdminError(sess, deps, "全玩家存檔未全部完成", err)
		return
	}
	gmMsgf(sess, "已儲存所有線上玩家（%d 人）。", deps.World.PlayerCount())
}

func gmAdminPresent(sess *net.Session, cmd string, args []string, deps *Deps) {
	if deps.Items == nil || deps.WarehouseRepo == nil {
		gmMsg(sess, "贈品服務尚未初始化。")
		return
	}
	request, err := parseGMGiftRequest(cmd, args, deps.Items)
	if err != nil {
		gmMsg(sess, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accounts, err := deps.WarehouseRepo.GiveGMGift(ctx, request)
	if err != nil {
		gmAdminError(sess, deps, "倉庫贈品發送失敗", err)
		return
	}
	if accounts == 0 {
		gmMsg(sess, "沒有符合條件的帳號，未發送贈品。")
		return
	}
	gmMsgf(sess, "%s x%d 已發送至 %d 個帳號的個人倉庫。", deps.Items.Get(request.ItemID).Name, request.Count, accounts)
}

func parseGMGiftRequest(cmd string, args []string, items *data.ItemTable) (persist.GMGiftRequest, error) {
	var empty persist.GMGiftRequest
	levelGift := cmd == "lvpresent"
	expected := 4
	if levelGift {
		expected = 5
	}
	usage := "用法: .present 帳號|* 道具編號 強化等級 數量"
	if levelGift {
		usage = "用法: .lvpresent 最低等級 最高等級 道具編號 強化等級 數量"
	}
	if len(args) != expected {
		return empty, fmt.Errorf("%s", usage)
	}
	offset := expected - 3
	itemID, e1 := strconv.ParseInt(args[offset], 10, 32)
	enchant, e2 := strconv.ParseInt(args[offset+1], 10, 8)
	count, e3 := strconv.ParseInt(args[offset+2], 10, 32)
	if e1 != nil || e2 != nil || e3 != nil || itemID <= 0 || count <= 0 || enchant < -127 {
		return empty, fmt.Errorf("%s", usage)
	}
	item := items.Get(int32(itemID))
	if item == nil {
		return empty, fmt.Errorf("不存在的道具編號。")
	}
	request := persist.GMGiftRequest{Account: strings.ToLower(args[0]), ItemID: item.ItemID, Enchant: int16(enchant), Count: int32(count), Stackable: item.Stackable, Bless: int16(item.Bless), ChargeCount: int16(item.MaxChargeCount)}
	if levelGift {
		minLevel, errMin := strconv.ParseInt(args[0], 10, 16)
		maxLevel, errMax := strconv.ParseInt(args[1], 10, 16)
		if errMin != nil || errMax != nil || minLevel < 1 || maxLevel < minLevel {
			return empty, fmt.Errorf("%s", usage)
		}
		request.ByLevel = true
		request.MinLevel, request.MaxLevel = int16(minLevel), int16(maxLevel)
	}
	return request, nil
}

func gmAdminCleaning(sess *net.Session, deps *Deps) {
	removed := 0
	failed := 0
	deps.World.AllGroundItems(func(item *world.GroundItem) {
		// Java getVisiblePlayer(item,0)：玩家踩著該地面物時保留，附近其他物品仍能清理。
		occupied := false
		deps.World.AllPlayers(func(p *world.PlayerInfo) {
			if p.X == item.X && p.Y == item.Y && p.MapID == item.MapID && p.ShowID == item.ShowID {
				occupied = true
			}
		})
		if occupied || item.X == 0 && item.Y == 0 {
			return
		}
		if item.ItemID == 40314 || item.ItemID == 40316 {
			if deps.PetRepo == nil {
				failed++
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			objectID := item.ID
			if item.Item != nil {
				objectID = item.Item.ObjectID
			}
			err := deps.PetRepo.Delete(ctx, objectID)
			cancel()
			if err != nil {
				failed++
				return
			}
		}
		deps.World.RemoveGroundItem(item.ID)
		deps.World.AllPlayers(func(p *world.PlayerInfo) {
			if p.Known == nil {
				return
			}
			if _, ok := p.Known.GroundItems[item.ID]; ok {
				delete(p.Known.GroundItems, item.ID)
				if p.Session != nil {
					p.Session.Send(BuildRemoveObject(item.ID))
				}
			}
		})
		removed++
	})
	gmBroadcast(deps, "地上的垃圾被 GM 清除了。")
	gmMsgf(sess, "清除 %d 件地面物品；%d 件相關資料未能清除，已保留原物品。", removed, failed)
}

func gmAdminHomeTown(sess *net.Session, args []string, deps *Deps) {
	if len(args) != 1 || !strings.EqualFold(args[0], "daily") && !strings.EqualFold(args[0], "monthly") {
		gmMsg(sess, "用法: .hometown daily|monthly")
		return
	}
	if deps.CharRepo == nil {
		gmMsg(sess, "城鎮資料庫尚未初始化。")
		return
	}
	monthly := strings.EqualFold(args[0], "monthly")
	if monthly {
		if gmAdminState(deps).SaveAll == nil {
			gmMsg(sess, "存檔服務尚未初始化，無法執行月結。")
			return
		}
		if err := deps.GMAdmin.SaveAll(); err != nil {
			gmAdminError(sess, deps, "城鎮月結前存檔失敗", err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := deps.CharRepo.ProcessHomeTown(ctx, monthly); err != nil {
		gmAdminError(sess, deps, "城鎮結算失敗", err)
		return
	}
	if monthly {
		notices, err := deps.CharRepo.HomeTownLeaderNotices(ctx)
		if err != nil {
			gmAdminError(sess, deps, "城鎮結算已完成，但村長通知失敗", err)
			return
		}
		deps.World.AllPlayers(func(p *world.PlayerInfo) {
			if name, ok := notices[p.CharID]; ok && p.Session != nil {
				w := packet.NewWriterWithOpcode(packet.S_OPCODE_EVENT)
				w.WriteC(23)
				w.WriteS(name)
				p.Session.Send(w.Bytes())
			}
		})
	}
	gmMsgf(sess, "城鎮 %s 結算完成。", args[0])
}

func gmAdminError(sess *net.Session, deps *Deps, message string, err error) {
	if deps.Log != nil {
		deps.Log.Error(message, zap.Error(err))
	}
	gmMsg(sess, message+"，請檢查伺服器日誌。")
}
