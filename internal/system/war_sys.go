package system

import (
	"strings"
	"time"

	coresys "github.com/l1jgo/server/internal/core/system"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// WarSystem 處理戰爭邏輯（宣戰/投降/休戰/攻城勝利）。
// 實作 handler.WarManager 介面。
//
// Java 對照：C_War（請求與檢查）、C_Attr case 217/221/222（對方盟主同意）、
// L1War（RequestSimWar / RequestCastleWar / SimWarTimer）。
type WarSystem struct {
	deps *handler.Deps
	wars []*handler.ActiveWar // 進行中的戰爭
	now  func() time.Time     // 測試可替換的時鐘
}

// 戰爭請求類型（Java C_War type）。
const (
	warReqDeclare   = 0 // 宣戰
	warReqSurrender = 2 // 投降
	warReqCease     = 3 // 休戰
)

// S_Message_YN 類型（Java C_War → S_Message_YN）。
const (
	warYNDeclare   uint16 = 217 // %0 血盟的 %1 想與你的血盟開戰，是否接受？
	warYNSurrender uint16 = 221 // %0 血盟希望投降，是否接受？
	warYNCease     uint16 = 222 // %0 血盟希望結束戰爭，是否接受？
)

// javaSimWarMinLevel 對應 Java C_War：`player.getLevel() <= 15` 不可宣布血盟戰（232）。
const javaSimWarMinLevel = 15

// NewWarSystem 建立戰爭系統。
func NewWarSystem(deps *handler.Deps) *WarSystem {
	return &WarSystem{
		deps: deps,
		wars: make([]*handler.ActiveWar, 0),
	}
}

// clock 取得目前時間（測試可替換）。
func (s *WarSystem) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// simWarDuration 模擬戰持續時間（Java L1War.SimWarTimer 固定 240 分鐘）。
func (s *WarSystem) simWarDuration() time.Duration {
	if s.deps != nil && s.deps.Config != nil && s.deps.Config.Gameplay.SimWarTime > 0 {
		return s.deps.Config.Gameplay.SimWarTime
	}
	return 240 * time.Minute
}

// DeclareWar 宣戰（Java: C_War type=0）。
func (s *WarSystem) DeclareWar(sess *net.Session, player *world.PlayerInfo, targetClanName string) {
	s.requestWar(sess, player, warReqDeclare, targetClanName)
}

// SurrenderWar 投降（Java: C_War type=2）。
func (s *WarSystem) SurrenderWar(sess *net.Session, player *world.PlayerInfo, targetClanName string) {
	s.requestWar(sess, player, warReqSurrender, targetClanName)
}

// CeaseWar 休戰（Java: C_War type=3）。
func (s *WarSystem) CeaseWar(sess *net.Session, player *world.PlayerInfo, targetClanName string) {
	s.requestWar(sess, player, warReqCease, targetClanName)
}

// requestWar 依 Java C_War 的順序檢查後分派到攻城戰或血盟戰流程。
func (s *WarSystem) requestWar(sess *net.Session, player *world.PlayerInfo, reqType int, targetClanName string) {
	if s.deps == nil || s.deps.World == nil || s.deps.World.Clans == nil {
		return
	}
	// 只有王族可以宣戰（478）
	if player.ClassType != 0 {
		handler.SendServerMessage(sess, 478)
		return
	}
	// 必須有血盟（272）
	if player.ClanID == 0 {
		handler.SendServerMessage(sess, 272)
		return
	}
	clan := s.deps.World.Clans.GetClan(player.ClanID)
	if clan == nil {
		return
	}
	// 必須是盟主（478）
	if clan.LeaderID != player.CharID {
		handler.SendServerMessage(sess, 478)
		return
	}
	// 指定自己的血盟 → 靜默忽略
	if strings.EqualFold(clan.ClanName, targetClanName) {
		return
	}
	enemyClan := s.deps.World.Clans.GetClanByName(targetClanName)
	if enemyClan == nil {
		return
	}

	// 自己血盟是否已在戰爭中：宣戰 → 234；投降／休戰則必須在戰爭中
	inWar := s.findWarWithClan(clan.ClanName) != nil
	if inWar && reqType == warReqDeclare {
		handler.SendServerMessage(sess, 234) // 你的血盟已在戰爭中
		return
	}
	if !inWar && reqType != warReqDeclare {
		return
	}

	// 已擁有城堡：不能再宣戰（474）；投降／休戰靜默忽略
	if clan.HasCastle != 0 {
		if reqType == warReqDeclare {
			handler.SendServerMessage(sess, 474)
		}
		return
	}

	// 血盟戰：君主等級 ≤ 15 不可宣戰（232）
	if enemyClan.HasCastle == 0 && int(player.Level) <= javaSimWarMinLevel {
		handler.SendServerMessage(sess, 232)
		return
	}
	// 攻城戰：君主等級 < 25 不可宣戰（475）
	if enemyClan.HasCastle != 0 && player.Level < 25 {
		handler.SendServerMessage(sess, 475)
		return
	}

	if enemyClan.HasCastle != 0 {
		s.requestCastleWar(sess, player, clan, enemyClan, reqType)
	} else {
		s.requestSimWar(sess, player, clan, enemyClan, reqType)
	}
}

// requestCastleWar 攻城戰請求（Java C_War 相手クランが城主 分支）。
func (s *WarSystem) requestCastleWar(sess *net.Session, player *world.PlayerInfo, clan, enemyClan *world.ClanInfo, reqType int) {
	castleID := enemyClan.HasCastle

	// 非攻城戰時間：宣戰 → 476；投降／休戰靜默忽略
	if s.deps.Castle == nil || !s.deps.Castle.IsWarNow(castleID) {
		if reqType == warReqDeclare {
			handler.SendServerMessage(sess, 476)
		}
		return
	}

	// 所有線上盟員必須在城外（477）
	blocked := false
	s.deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if blocked || p.ClanID != clan.ClanID {
			return
		}
		if s.deps.Castle.CheckInWarArea(castleID, p.X, p.Y, p.MapID) {
			blocked = true
		}
	})
	if blocked {
		handler.SendServerMessage(sess, 477)
		return
	}

	existing := s.findWarWithClan(enemyClan.ClanName)
	if existing != nil {
		switch reqType {
		case warReqDeclare:
			existing.AttackClans[clan.ClanName] = true
			// 廣播宣戰訊息（Java: S_War type=1 → msgID 226）
			handler.BroadcastWarPacket(s.deps.World, 1, clan.ClanName, enemyClan.ClanName)
		case warReqSurrender, warReqCease:
			if !s.clanInWar(existing, clan.ClanName) {
				return
			}
			s.leaveCastleWar(existing, clan.ClanName, enemyClan.ClanName, byte(reqType))
		}
		return
	}

	if reqType != warReqDeclare {
		return
	}
	w := &handler.ActiveWar{
		WarType:     1,
		DefenceClan: enemyClan.ClanName,
		AttackClans: map[string]bool{clan.ClanName: true},
		CastleID:    castleID,
		StartTime:   s.clock(),
	}
	s.wars = append(s.wars, w)
	handler.BroadcastWarPacket(s.deps.World, 1, clan.ClanName, enemyClan.ClanName)
}

// leaveCastleWar 攻擊方投降／休戰離開攻城戰（Go 既有行為：只移除該攻擊方，其他攻擊方續戰）。
func (s *WarSystem) leaveCastleWar(w *handler.ActiveWar, clanName, targetClanName string, warType byte) {
	delete(w.AttackClans, clanName)
	// Java: S_War type=2 → msgID 228（投降）、type=3 → msgID 227（終結）
	handler.BroadcastWarPacket(s.deps.World, warType, clanName, targetClanName)
	if len(w.AttackClans) == 0 {
		s.removeWar(w)
	}
}

// requestSimWar 血盟戰請求：需對方盟主在線並以 S_Message_YN 同意（Java C_War 相手クランが城主ではない 分支）。
func (s *WarSystem) requestSimWar(sess *net.Session, player *world.PlayerInfo, clan, enemyClan *world.ClanInfo, reqType int) {
	existing := s.findWarWithClan(enemyClan.ClanName)
	if existing != nil {
		if reqType == warReqDeclare {
			handler.SendServerMessageArgs(sess, 236, enemyClan.ClanName) // %0 血盟拒絕與你的血盟開戰
			return
		}
		if !s.clanInWar(existing, clan.ClanName) {
			return
		}
	} else if reqType != warReqDeclare {
		return
	}

	enemyLeader := s.deps.World.GetByCharID(enemyClan.LeaderID)
	if enemyLeader == nil && enemyClan.LeaderName != "" {
		enemyLeader = s.deps.World.GetByName(enemyClan.LeaderName)
	}
	if enemyLeader == nil || enemyLeader.Session == nil {
		handler.SendServerMessageArgs(sess, 218, enemyClan.ClanName) // %0 血盟的君主目前不在線上
		return
	}

	// Java: enemyLeader.setTempID(player.getId()) 後送 S_Message_YN
	enemyLeader.TempID = player.CharID
	switch reqType {
	case warReqDeclare:
		handler.SendYesNoDialog(enemyLeader.Session, warYNDeclare, clan.ClanName, player.Name)
	case warReqSurrender:
		handler.SendYesNoDialog(enemyLeader.Session, warYNSurrender, clan.ClanName)
	case warReqCease:
		handler.SendYesNoDialog(enemyLeader.Session, warYNCease, clan.ClanName)
	}
}

// HandleWarConsent 對方盟主回應宣戰／投降／休戰（Java: C_Attr case 217/221/222）。
func (s *WarSystem) HandleWarConsent(sess *net.Session, player *world.PlayerInfo, msgType uint16, accepted bool) {
	if s.deps == nil || s.deps.World == nil || s.deps.World.Clans == nil {
		return
	}
	enemyLeader := s.deps.World.GetByCharID(player.TempID)
	if enemyLeader == nil {
		return
	}
	player.TempID = 0

	clan := s.deps.World.Clans.GetClan(player.ClanID)
	enemyClan := s.deps.World.Clans.GetClan(enemyLeader.ClanID)
	if clan == nil || enemyClan == nil {
		return
	}
	clanName := clan.ClanName
	enemyClanName := enemyClan.ClanName

	if !accepted {
		if enemyLeader.Session == nil {
			return
		}
		switch msgType {
		case warYNDeclare:
			handler.SendServerMessageArgs(enemyLeader.Session, 236, clanName) // %0 血盟拒絕你的宣戰
		case warYNSurrender, warYNCease:
			handler.SendServerMessageArgs(enemyLeader.Session, 237, clanName) // %0 血盟拒絕你的提案
		}
		return
	}

	switch msgType {
	case warYNDeclare:
		// Java 同意時不再重查；Go 加一道保險避免等待同意期間另一場戰爭已成立
		if s.findWarWithClan(clanName) != nil || s.findWarWithClan(enemyClanName) != nil {
			if s.deps.Log != nil {
				s.deps.Log.Info("血盟戰同意時其中一方已在戰爭中，忽略",
					zap.String("attack", enemyClanName), zap.String("defence", clanName))
			}
			return
		}
		s.startSimWar(enemyClanName, clanName)
	case warYNSurrender, warYNCease:
		w := s.findWarWithClan(clanName)
		if w == nil || w.WarType != 2 {
			return
		}
		if msgType == warYNSurrender {
			s.endSimWar(w, 2, enemyClanName, clanName)
		} else {
			s.endSimWar(w, 3, enemyClanName, clanName)
		}
	}
}

// startSimWar 建立血盟戰（Java: L1War.handleCommands(2, attack, defence) → RequestSimWar(1)）。
func (s *WarSystem) startSimWar(attackClanName, defenceClanName string) {
	now := s.clock()
	w := &handler.ActiveWar{
		WarType:     2,
		DefenceClan: defenceClanName,
		AttackClans: map[string]bool{attackClanName: true},
		CastleID:    0,
		StartTime:   now,
		EndTime:     now.Add(s.simWarDuration()),
	}
	s.wars = append(s.wars, w)
	// Java RequestSimWar(1)：雙方盟員各收 S_War(1, attack, defence)
	s.sendWarPacketToClan(attackClanName, 1, attackClanName, defenceClanName)
	s.sendWarPacketToClan(defenceClanName, 1, attackClanName, defenceClanName)
}

// endSimWar 結束血盟戰（Java: L1War.RequestSimWar type=2 投降／3 終結）。
// clan1 為提出方（投降者／休戰提出者或計時到期時的宣戰方），clan2 為對方。
func (s *WarSystem) endSimWar(w *handler.ActiveWar, warType byte, clan1, clan2 string) {
	s.removeWar(w)
	// clan1 盟員：S_War(type, clan1, clan2)
	s.sendWarPacketToClan(clan1, warType, clan1, clan2)
	// clan2 盟員：S_War(type, clan1, clan2) + S_War(4, clan2, clan1)
	s.sendWarPacketToClan(clan2, warType, clan1, clan2)
	s.sendWarPacketToClan(clan2, 4, clan2, clan1)
}

// TickSimWar 血盟戰到期自動休戰（Java: L1War.SimWarTimer 240 分鐘後 CeaseWar(attack, defence)）。
func (s *WarSystem) TickSimWar() {
	now := s.clock()
	for i := len(s.wars) - 1; i >= 0; i-- {
		w := s.wars[i]
		if w.WarType != 2 || w.EndTime.IsZero() || !w.EndTime.Before(now) {
			continue
		}
		attack := firstAttackClan(w)
		if s.deps != nil && s.deps.Log != nil {
			s.deps.Log.Info("血盟戰時間到，自動休戰",
				zap.String("attack", attack), zap.String("defence", w.DefenceClan))
		}
		s.endSimWar(w, 3, attack, w.DefenceClan)
	}
}

// firstAttackClan 取攻擊方清單第一個（Java _param1；模擬戰只有一個攻擊方）。
func firstAttackClan(w *handler.ActiveWar) string {
	for name := range w.AttackClans {
		return name
	}
	return ""
}

// sendWarPacketToClan 對指定血盟的線上盟員送 S_War（Java: clan.getOnlineClanMember() 逐一 sendPackets）。
func (s *WarSystem) sendWarPacketToClan(clanName string, warType byte, clan1, clan2 string) {
	clan := s.deps.World.Clans.GetClanByName(clanName)
	if clan == nil {
		return
	}
	data := handler.BuildWarPacket(warType, clan1, clan2)
	s.deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if p.ClanID == clan.ClanID && p.Session != nil {
			p.Session.Send(data)
		}
	})
}

// removeWar 從戰爭清單移除。
func (s *WarSystem) removeWar(target *handler.ActiveWar) {
	for i, w := range s.wars {
		if w == target {
			s.wars = append(s.wars[:i], s.wars[i+1:]...)
			return
		}
	}
}

// clanInWar 血盟是否參與該場戰爭（Java: L1War.CheckClanInWar）。
func (s *WarSystem) clanInWar(w *handler.ActiveWar, clanName string) bool {
	if strings.EqualFold(w.DefenceClan, clanName) {
		return true
	}
	for name := range w.AttackClans {
		if strings.EqualFold(name, clanName) {
			return true
		}
	}
	return false
}

// findWarWithClan 找出血盟參與中的第一場戰爭。
func (s *WarSystem) findWarWithClan(clanName string) *handler.ActiveWar {
	for _, w := range s.wars {
		if s.clanInWar(w, clanName) {
			return w
		}
	}
	return nil
}

// WinCastleWar 攻城勝利（王冠被取得）。
func (s *WarSystem) WinCastleWar(winnerClanName string, castleID int32) {
	// 廣播勝利訊息（Java: S_War type=4 → msgID 231）
	for i := len(s.wars) - 1; i >= 0; i-- {
		w := s.wars[i]
		if w.WarType == 1 && w.CastleID == castleID {
			// 對所有參戰方廣播結束
			for attackClan := range w.AttackClans {
				handler.BroadcastWarPacket(s.deps.World, 3, attackClan, w.DefenceClan)
			}
			s.wars = append(s.wars[:i], s.wars[i+1:]...)
		}
	}

	// 廣播佔領城堡訊息
	handler.BroadcastPacketBoxWar(s.deps.World, 4, castleID) // MSG_WAR_OCCUPY
}

// CeaseCastleWar 攻城戰時間到（防禦方勝利）。
func (s *WarSystem) CeaseCastleWar(castleID int32) {
	for i := len(s.wars) - 1; i >= 0; i-- {
		w := s.wars[i]
		if w.WarType == 1 && w.CastleID == castleID {
			for attackClan := range w.AttackClans {
				handler.BroadcastWarPacket(s.deps.World, 3, attackClan, w.DefenceClan)
			}
			s.wars = append(s.wars[:i], s.wars[i+1:]...)
		}
	}
}

// IsWar 兩個公會是否在戰爭中。
func (s *WarSystem) IsWar(clan1, clan2 string) bool {
	for _, w := range s.wars {
		if s.clanInWar(w, clan1) && s.clanInWar(w, clan2) {
			return true
		}
	}
	return false
}

// IsClanInWar 公會是否在任何戰爭中。
func (s *WarSystem) IsClanInWar(clanName string) bool {
	return s.findWarWithClan(clanName) != nil
}

// GetActiveWars 取得所有進行中的戰爭。
func (s *WarSystem) GetActiveWars() []*handler.ActiveWar {
	return s.wars
}

// CheckCastleWar 登入時通知攻城戰狀態（Java: ServerWarExecutor.checkCastleWar）。
func (s *WarSystem) CheckCastleWar(sess *net.Session) {
	if s.deps.Castle == nil {
		return
	}
	for i := int32(1); i <= 8; i++ {
		if s.deps.Castle.IsWarNow(i) {
			// MSG_WAR_GOING = 2（Java: S_PacketBoxWar(MSG_WAR_GOING, castleId)）
			handler.SendPacketBoxWarToPlayer(sess, 2, i)
		}
	}
}

// --- 血盟戰到期 tick ---

// SimWarTickSystem 每 tick 檢查血盟戰是否到期（Java: L1War.SimWarTimer）。
type SimWarTickSystem struct {
	war handler.WarManager
}

// NewSimWarTickSystem 建立血盟戰到期 tick 系統。
func NewSimWarTickSystem(war handler.WarManager) *SimWarTickSystem {
	return &SimWarTickSystem{war: war}
}

// Phase 回傳 Phase 3 (PostUpdate)。
func (s *SimWarTickSystem) Phase() coresys.Phase {
	return coresys.PhasePostUpdate
}

// Update 每 tick 呼叫 TickSimWar。
func (s *SimWarTickSystem) Update(dt time.Duration) {
	if s.war != nil {
		s.war.TickSimWar()
	}
}
