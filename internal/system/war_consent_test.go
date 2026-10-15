package system

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// --- 測試骨架 ---

type warTestEnv struct {
	t      *testing.T
	ws     *world.State
	deps   *handler.Deps
	war    *WarSystem
	castle *CastleSystem
	now    time.Time
	nextID uint64
}

func newWarTestEnv(t *testing.T) *warTestEnv {
	t.Helper()
	castle := newTestCastleSystem(t)
	deps := castle.deps
	deps.Castle = castle
	deps.Config = &config.Config{}
	env := &warTestEnv{
		t:      t,
		ws:     deps.World,
		deps:   deps,
		castle: castle,
		now:    time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local),
	}
	env.war = NewWarSystem(deps)
	env.war.now = func() time.Time { return env.now }
	castle.now = func() time.Time { return env.now }
	deps.War = env.war
	return env
}

func (e *warTestEnv) newSession() *l1net.Session {
	e.t.Helper()
	client, server := net.Pipe()
	e.t.Cleanup(func() { _ = client.Close() })
	e.nextID++
	sess := l1net.NewSession(server, e.nextID, 64, 64, 0, zap.NewNop())
	e.t.Cleanup(sess.Close)
	return sess
}

// addPlayer 建立線上玩家（ClassType 0 = 王族）。
func (e *warTestEnv) addPlayer(charID int32, name string, clanID int32, level int16) *world.PlayerInfo {
	sess := e.newSession()
	p := &world.PlayerInfo{
		SessionID: sess.ID,
		Session:   sess,
		CharID:    charID,
		Name:      name,
		ClanID:    clanID,
		Level:     level,
		ClassType: 0,
		X:         32600, Y: 32700, MapID: 4,
	}
	e.ws.AddPlayer(p)
	return p
}

// addClan 建立血盟並把 leader 設為盟主。
func (e *warTestEnv) addClan(clanID int32, name string, leader *world.PlayerInfo, castleID int32) *world.ClanInfo {
	c := &world.ClanInfo{
		ClanID:     clanID,
		ClanName:   name,
		LeaderID:   leader.CharID,
		LeaderName: leader.Name,
		HasCastle:  castleID,
		Members:    map[int32]*world.ClanMember{leader.CharID: {CharID: leader.CharID, CharName: leader.Name}},
	}
	e.ws.Clans.AddClan(c)
	leader.ClanID = clanID
	leader.ClanName = name
	return c
}

func drainWarPackets(p *world.PlayerInfo) [][]byte {
	p.Session.FlushOutput()
	var out [][]byte
	for {
		select {
		case pkt := <-p.Session.OutQueue:
			out = append(out, pkt)
		default:
			return out
		}
	}
}

// serverMessageIDs 取出所有 S_ServerMessage 的 msgID。
func serverMessageIDs(pkts [][]byte) []uint16 {
	var ids []uint16
	for _, pkt := range pkts {
		if len(pkt) >= 3 && pkt[0] == packet.S_OPCODE_MESSAGE_CODE {
			ids = append(ids, binary.LittleEndian.Uint16(pkt[1:3]))
		}
	}
	return ids
}

// yesNoTypes 取出所有 S_Message_YN 的 type（格式：op + H 0 + D count + H type + S...）。
func yesNoTypes(pkts [][]byte) []uint16 {
	var types []uint16
	for _, pkt := range pkts {
		if len(pkt) >= 9 && pkt[0] == packet.S_OPCODE_YES_NO {
			types = append(types, binary.LittleEndian.Uint16(pkt[7:9]))
		}
	}
	return types
}

// warPacketTypes 取出所有 S_War 的 type。
func warPacketTypes(pkts [][]byte) []byte {
	var types []byte
	for _, pkt := range pkts {
		if len(pkt) >= 2 && pkt[0] == packet.S_OPCODE_WAR {
			types = append(types, pkt[1])
		}
	}
	return types
}

func containsU16(list []uint16, v uint16) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsByte(list []byte, v byte) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// twoClans 建立 A（攻方）與 B（守方）兩個血盟，盟主皆線上且等級 50。
func twoClans(t *testing.T) (*warTestEnv, *world.PlayerInfo, *world.PlayerInfo) {
	env := newWarTestEnv(t)
	a := env.addPlayer(1001, "LeaderA", 0, 50)
	b := env.addPlayer(1002, "LeaderB", 0, 50)
	env.addClan(1, "ClanA", a, 0)
	env.addClan(2, "ClanB", b, 0)
	drainWarPackets(a)
	drainWarPackets(b)
	return env, a, b
}

// --- 1-6 宣戰需對方盟主同意 ---

func TestSimWarDeclareSendsYesNoInsteadOfStarting(t *testing.T) {
	env, a, b := twoClans(t)

	env.war.DeclareWar(a.Session, a, "ClanB")

	if len(env.war.GetActiveWars()) != 0 {
		t.Fatalf("宣戰不應直接成立，wars=%d", len(env.war.GetActiveWars()))
	}
	req := env.war.pending[b.CharID]
	if req == nil || req.requesterCharID != a.CharID || req.msgType != 217 || req.requesterClanID != 1 || req.responderClanID != 2 {
		t.Fatalf("應記錄綁定雙方的 pending 請求，got %+v", req)
	}
	if b.TempID != 0 {
		t.Fatalf("不應佔用通用 TempID，got %d", b.TempID)
	}
	bp := drainWarPackets(b)
	if !containsU16(yesNoTypes(bp), 217) {
		t.Fatalf("對方盟主應收到 S_Message_YN 217，got %v", bp)
	}
	if ids := serverMessageIDs(drainWarPackets(a)); len(ids) != 0 {
		t.Fatalf("請求者不應收到錯誤訊息，got %v", ids)
	}
}

func TestSimWarConsentYesStartsWarWithEndTime(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)

	env.war.HandleWarConsent(b.Session, b, 217, true)

	wars := env.war.GetActiveWars()
	if len(wars) != 1 {
		t.Fatalf("同意後應成立一場戰爭，got %d", len(wars))
	}
	w := wars[0]
	if w.WarType != 2 || w.DefenceClan != "ClanB" || !w.AttackClans["ClanA"] {
		t.Fatalf("戰爭內容錯誤: %+v", w)
	}
	if want := env.now.Add(240 * time.Minute); !w.EndTime.Equal(want) {
		t.Fatalf("EndTime 應為 240 分鐘後 %v，got %v", want, w.EndTime)
	}
	if env.war.pending[b.CharID] != nil {
		t.Fatal("回應後 pending 應清除")
	}
	if !containsByte(warPacketTypes(drainWarPackets(a)), 1) {
		t.Fatal("攻方盟員應收到 S_War type=1")
	}
	if !containsByte(warPacketTypes(drainWarPackets(b)), 1) {
		t.Fatal("守方盟員應收到 S_War type=1")
	}
	if !env.war.IsWar("ClanA", "ClanB") {
		t.Fatal("IsWar 應為 true")
	}
}

func TestSimWarConsentNoSends236(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)

	env.war.HandleWarConsent(b.Session, b, 217, false)

	if len(env.war.GetActiveWars()) != 0 {
		t.Fatal("拒絕後不應成立戰爭")
	}
	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 236) {
		t.Fatalf("請求者應收到 236，got %v", ids)
	}
	if env.war.pending[b.CharID] != nil {
		t.Fatal("回應後 pending 應清除")
	}
}

func TestSimWarDeclareLeaderOfflineSends218(t *testing.T) {
	env, a, b := twoClans(t)
	env.ws.RemovePlayer(b.SessionID)

	env.war.DeclareWar(a.Session, a, "ClanB")

	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 218) {
		t.Fatalf("對方盟主離線應回 218，got %v", ids)
	}
	if len(env.war.GetActiveWars()) != 0 {
		t.Fatal("不應成立戰爭")
	}
}

func TestSimWarDeclareLevel15Sends232(t *testing.T) {
	env, a, b := twoClans(t)
	a.Level = 15

	env.war.DeclareWar(a.Session, a, "ClanB")

	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 232) {
		t.Fatalf("等級 ≤ 15 應回 232，got %v", ids)
	}
	if yn := yesNoTypes(drainWarPackets(b)); len(yn) != 0 {
		t.Fatalf("對方不應收到詢問，got %v", yn)
	}
	// 16 級可以宣戰
	a.Level = 16
	env.war.DeclareWar(a.Session, a, "ClanB")
	if yn := yesNoTypes(drainWarPackets(b)); !containsU16(yn, 217) {
		t.Fatalf("16 級應可宣戰，got %v", yn)
	}
}

func TestSimWarDeclareChecksLeaderAndCrown(t *testing.T) {
	env, a, b := twoClans(t)

	// 非王族 → 478
	a.ClassType = 1
	env.war.DeclareWar(a.Session, a, "ClanB")
	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 478) {
		t.Fatalf("非王族應回 478，got %v", ids)
	}
	a.ClassType = 0

	// 無血盟 → 272
	member := env.addPlayer(1003, "NoClan", 0, 50)
	env.war.DeclareWar(member.Session, member, "ClanB")
	if ids := serverMessageIDs(drainWarPackets(member)); !containsU16(ids, 272) {
		t.Fatalf("無血盟應回 272，got %v", ids)
	}

	// 有血盟但非盟主 → 478
	member.ClanID = 1
	env.war.DeclareWar(member.Session, member, "ClanB")
	if ids := serverMessageIDs(drainWarPackets(member)); !containsU16(ids, 478) {
		t.Fatalf("非盟主應回 478，got %v", ids)
	}

	// 指定自己血盟 → 靜默
	env.war.DeclareWar(a.Session, a, "clana")
	if pk := drainWarPackets(a); len(pk) != 0 {
		t.Fatalf("指定自己血盟應靜默，got %d packets", len(pk))
	}
	if yn := yesNoTypes(drainWarPackets(b)); len(yn) != 0 {
		t.Fatalf("對方不應收到詢問，got %v", yn)
	}
}

func TestSimWarDeclareWhenTargetAlreadyInWarSends236(t *testing.T) {
	env, a, b := twoClans(t)
	c := env.addPlayer(1004, "LeaderC", 0, 50)
	env.addClan(3, "ClanC", c, 0)
	env.war.startSimWar("ClanC", "ClanB")
	drainWarPackets(b)

	env.war.DeclareWar(a.Session, a, "ClanB")

	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 236) {
		t.Fatalf("對方已在戰爭中應回 236，got %v", ids)
	}
	if yn := yesNoTypes(drainWarPackets(b)); len(yn) != 0 {
		t.Fatalf("對方不應收到詢問，got %v", yn)
	}
}

func TestSimWarDeclareWhenSelfAlreadyInWarSends234(t *testing.T) {
	env, a, b := twoClans(t)
	c := env.addPlayer(1004, "LeaderC", 0, 50)
	env.addClan(3, "ClanC", c, 0)
	env.war.startSimWar("ClanA", "ClanC")
	drainWarPackets(a)

	env.war.DeclareWar(a.Session, a, "ClanB")

	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 234) {
		t.Fatalf("自己已在戰爭中應回 234，got %v", ids)
	}
	if yn := yesNoTypes(drainWarPackets(b)); len(yn) != 0 {
		t.Fatalf("對方不應收到詢問，got %v", yn)
	}
}

// 重複請求：同一請求者同類型重送只保留一筆 pending，回覆只生效一次。
func TestSimWarDuplicateRequestOnlyStartsOneWar(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	env.war.DeclareWar(a.Session, a, "ClanB")
	if yn := yesNoTypes(drainWarPackets(b)); len(yn) != 2 {
		t.Fatalf("應收到兩次詢問，got %v", yn)
	}

	env.war.HandleWarConsent(b.Session, b, 217, true)
	// 第二個對話框的回覆：pending 已消耗 → 忽略
	env.war.HandleWarConsent(b.Session, b, 217, true)

	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("重複回覆只應成立一場戰爭，got %d", n)
	}
}

// 過期回覆：請求者在對方回覆前已離線 → 靜默忽略。
func TestSimWarStaleReplyRequesterOffline(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	env.ws.RemovePlayer(a.SessionID)

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if len(env.war.GetActiveWars()) != 0 {
		t.Fatal("請求者離線後的回覆不應成立戰爭")
	}
}

// 過期回覆：等待同意期間對方已與第三盟開戰 → 同意時不再成立第二場。
func TestSimWarStaleReplyWhenAlreadyInAnotherWar(t *testing.T) {
	env, a, b := twoClans(t)
	c := env.addPlayer(1004, "LeaderC", 0, 50)
	env.addClan(3, "ClanC", c, 0)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	// A 在等待期間先與 C 開戰
	env.war.startSimWar("ClanA", "ClanC")

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("不應成立第二場戰爭，got %d", n)
	}
	if env.war.IsWar("ClanA", "ClanB") {
		t.Fatal("A-B 不應成立戰爭")
	}
}

// 換盟：請求者在回覆前退盟／換盟 → 以回覆當下的血盟為準。
func TestSimWarReplyAfterRequesterLeftClan(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	a.ClanID = 0

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if len(env.war.GetActiveWars()) != 0 {
		t.Fatal("請求者已無血盟，不應成立戰爭")
	}
}

// 無 pending 的回覆（例如客戶端偽造）→ 靜默。
func TestSimWarConsentWithoutPendingRequestIgnored(t *testing.T) {
	env, a, b := twoClans(t)
	b.TempID = a.CharID // 即使通用 TempID 被其他對話設成請求者，也不得生效

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if len(env.war.GetActiveWars()) != 0 {
		t.Fatal("無待處理請求不應成立戰爭")
	}
}

// --- 1-6 投降／休戰需對方同意 ---

func TestSimWarSurrenderRequiresConsent(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	drainWarPackets(a)
	drainWarPackets(b)

	env.war.SurrenderWar(a.Session, a, "ClanB")
	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("投降請求不應直接結束戰爭，got %d", n)
	}
	if yn := yesNoTypes(drainWarPackets(b)); !containsU16(yn, 221) {
		t.Fatalf("對方應收到 221，got %v", yn)
	}

	// 拒絕 → 237，戰爭持續
	env.war.HandleWarConsent(b.Session, b, 221, false)
	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 237) {
		t.Fatalf("拒絕應回 237，got %v", ids)
	}
	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("拒絕後戰爭應持續，got %d", n)
	}

	// 再次請求並同意 → 結束，守方收 S_War 2 + 4
	env.war.SurrenderWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	env.war.HandleWarConsent(b.Session, b, 221, true)
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("同意投降後戰爭應結束，got %d", n)
	}
	at := warPacketTypes(drainWarPackets(a))
	if !containsByte(at, 2) {
		t.Fatalf("投降方應收 S_War 2，got %v", at)
	}
	bt := warPacketTypes(drainWarPackets(b))
	if !containsByte(bt, 2) || !containsByte(bt, 4) {
		t.Fatalf("對方應收 S_War 2 與 4，got %v", bt)
	}
}

func TestSimWarCeaseRequiresConsent(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	drainWarPackets(a)
	drainWarPackets(b)

	// 由守方提出休戰亦可
	env.war.CeaseWar(b.Session, b, "ClanA")
	if yn := yesNoTypes(drainWarPackets(a)); !containsU16(yn, 222) {
		t.Fatalf("對方應收到 222，got %v", yn)
	}
	if req := env.war.pending[a.CharID]; req == nil || req.requesterCharID != b.CharID || req.msgType != 222 || req.war != env.war.GetActiveWars()[0] {
		t.Fatalf("休戰請求應綁定提出者與原戰爭，got %+v", req)
	}

	env.war.HandleWarConsent(a.Session, a, 222, true)
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("同意休戰後戰爭應結束，got %d", n)
	}
	bt := warPacketTypes(drainWarPackets(b))
	if !containsByte(bt, 3) {
		t.Fatalf("提出方應收 S_War 3，got %v", bt)
	}
	at := warPacketTypes(drainWarPackets(a))
	if !containsByte(at, 3) || !containsByte(at, 4) {
		t.Fatalf("同意方應收 S_War 3 與 4，got %v", at)
	}
}

func TestSimWarSurrenderOrCeaseWhenNotInWarIgnored(t *testing.T) {
	env, a, b := twoClans(t)

	env.war.SurrenderWar(a.Session, a, "ClanB")
	env.war.CeaseWar(a.Session, a, "ClanB")

	if pk := drainWarPackets(b); len(pk) != 0 {
		t.Fatalf("未在戰爭中不應詢問對方，got %d packets", len(pk))
	}
	if pk := drainWarPackets(a); len(pk) != 0 {
		t.Fatalf("未在戰爭中應靜默，got %d packets", len(pk))
	}
}

// 投降／休戰對象不在同一場戰爭 → 靜默。
func TestSimWarSurrenderToClanInDifferentWarIgnored(t *testing.T) {
	env, a, _ := twoClans(t)
	c := env.addPlayer(1004, "LeaderC", 0, 50)
	d := env.addPlayer(1005, "LeaderD", 0, 50)
	env.addClan(3, "ClanC", c, 0)
	env.addClan(4, "ClanD", d, 0)
	env.war.startSimWar("ClanA", "ClanB")
	env.war.startSimWar("ClanC", "ClanD")
	drainWarPackets(c)

	env.war.SurrenderWar(a.Session, a, "ClanC")

	if yn := yesNoTypes(drainWarPackets(c)); len(yn) != 0 {
		t.Fatalf("不同戰爭不應詢問，got %v", yn)
	}
	if n := len(env.war.GetActiveWars()); n != 2 {
		t.Fatalf("兩場戰爭都應持續，got %d", n)
	}
}

// --- 1-5 模擬戰 240 分鐘自動休戰 ---

func TestSimWarAutoCeaseAt240Minutes(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	drainWarPackets(a)
	drainWarPackets(b)

	// 239 分 59 秒：仍在戰爭中
	env.now = env.now.Add(240*time.Minute - time.Second)
	env.war.TickSimWar()
	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("到期前不應結束，got %d", n)
	}

	// 恰好 240 分：Java 條件為 EndTime.before(now)，相等不觸發
	env.now = env.now.Add(time.Second)
	env.war.TickSimWar()
	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("恰好到期時刻不應結束，got %d", n)
	}

	// 240 分 + 1 秒：結束並通知
	env.now = env.now.Add(time.Second)
	env.war.TickSimWar()
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("到期後應自動休戰，got %d", n)
	}
	at := warPacketTypes(drainWarPackets(a))
	if !containsByte(at, 3) {
		t.Fatalf("攻方應收 S_War 3，got %v", at)
	}
	bt := warPacketTypes(drainWarPackets(b))
	if !containsByte(bt, 3) || !containsByte(bt, 4) {
		t.Fatalf("守方應收 S_War 3 與 4，got %v", bt)
	}
	if env.war.IsWar("ClanA", "ClanB") {
		t.Fatal("休戰後 IsWar 應為 false")
	}

	// 再 tick 一次不應重複送封包
	env.war.TickSimWar()
	if pk := drainWarPackets(b); len(pk) != 0 {
		t.Fatalf("結束後不應再送封包，got %d", len(pk))
	}
}

func TestSimWarTickDoesNotTouchCastleWar(t *testing.T) {
	env, _, _ := twoClans(t)
	env.war.wars = append(env.war.wars, &handler.ActiveWar{
		WarType:     1,
		DefenceClan: "ClanB",
		AttackClans: map[string]bool{"ClanA": true},
		CastleID:    1,
		StartTime:   env.now,
	})

	env.now = env.now.Add(48 * time.Hour)
	env.war.TickSimWar()

	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("攻城戰不受模擬戰計時影響，got %d", n)
	}
}

func TestSimWarDurationConfigOverride(t *testing.T) {
	env, _, _ := twoClans(t)
	env.deps.Config.Gameplay.SimWarTime = 5 * time.Minute
	env.war.startSimWar("ClanA", "ClanB")

	if want := env.now.Add(5 * time.Minute); !env.war.GetActiveWars()[0].EndTime.Equal(want) {
		t.Fatalf("EndTime 應採設定值，got %v", env.war.GetActiveWars()[0].EndTime)
	}
	env.now = env.now.Add(5*time.Minute + time.Second)
	env.war.TickSimWar()
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("設定時間到期應結束，got %d", n)
	}

	// 零值回退 240 分鐘
	env.deps.Config.Gameplay.SimWarTime = 0
	if got := env.war.simWarDuration(); got != 240*time.Minute {
		t.Fatalf("零值應回退 240m，got %v", got)
	}
}

func TestSimWarTickSystemCallsManager(t *testing.T) {
	env, _, _ := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	sys := NewSimWarTickSystem(env.war)
	env.now = env.now.Add(241 * time.Minute)

	sys.Update(0)

	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("tick system 應觸發自動休戰，got %d", n)
	}
}

// --- 攻城戰既有行為回歸 ---

func TestCastleWarDeclareStillImmediate(t *testing.T) {
	env, a, b := twoClans(t)
	env.ws.Clans.GetClan(2).HasCastle = 1
	env.castle.castles[1].OwnerClanID = 2
	env.castle.castles[1].IsWar = true
	drainWarPackets(b)

	env.war.DeclareWar(a.Session, a, "ClanB")

	wars := env.war.GetActiveWars()
	if len(wars) != 1 || wars[0].WarType != 1 || wars[0].CastleID != 1 || !wars[0].AttackClans["ClanA"] {
		t.Fatalf("攻城戰宣戰應立即成立，got %+v", wars)
	}
	if !wars[0].EndTime.IsZero() {
		t.Fatal("攻城戰不應設定模擬戰 EndTime")
	}
	if yn := yesNoTypes(drainWarPackets(b)); len(yn) != 0 {
		t.Fatalf("攻城戰不需對方同意，got %v", yn)
	}
	if !containsByte(warPacketTypes(drainWarPackets(a)), 1) {
		t.Fatal("應廣播 S_War 1")
	}

	// 投降：直接生效、不需同意
	env.war.SurrenderWar(a.Session, a, "ClanB")
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("攻城戰投降應直接結束，got %d", n)
	}
	if !containsByte(warPacketTypes(drainWarPackets(a)), 2) {
		t.Fatal("應廣播 S_War 2")
	}
}

func TestCastleWarDeclareChecks(t *testing.T) {
	env, a, b := twoClans(t)
	env.ws.Clans.GetClan(2).HasCastle = 1
	env.castle.castles[1].OwnerClanID = 2

	// 非攻城時間 → 476
	env.war.DeclareWar(a.Session, a, "ClanB")
	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 476) {
		t.Fatalf("非攻城時間應回 476，got %v", ids)
	}

	env.castle.castles[1].IsWar = true

	// 等級 < 25 → 475
	a.Level = 24
	env.war.DeclareWar(a.Session, a, "ClanB")
	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 475) {
		t.Fatalf("等級 < 25 應回 475，got %v", ids)
	}
	a.Level = 50

	// 盟員在戰區內 → 477
	member := env.addPlayer(1003, "MemberA", 1, 30)
	member.X, member.Y, member.MapID = 33150, 32770, 4
	env.war.DeclareWar(a.Session, a, "ClanB")
	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 477) {
		t.Fatalf("盟員在戰區內應回 477，got %v", ids)
	}
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("不應成立戰爭，got %d", n)
	}
	member.X, member.Y = 32600, 32700

	// 自己已有城堡 → 474
	env.ws.Clans.GetClan(1).HasCastle = 2
	env.war.DeclareWar(a.Session, a, "ClanB")
	if ids := serverMessageIDs(drainWarPackets(a)); !containsU16(ids, 474) {
		t.Fatalf("已有城堡應回 474，got %v", ids)
	}
	env.ws.Clans.GetClan(1).HasCastle = 0

	_ = b
}

func TestCastleWarSecondAttackerJoinsExistingWar(t *testing.T) {
	env, a, _ := twoClans(t)
	c := env.addPlayer(1004, "LeaderC", 0, 50)
	env.addClan(3, "ClanC", c, 0)
	env.ws.Clans.GetClan(2).HasCastle = 1
	env.castle.castles[1].OwnerClanID = 2
	env.castle.castles[1].IsWar = true

	env.war.DeclareWar(a.Session, a, "ClanB")
	env.war.DeclareWar(c.Session, c, "ClanB")

	wars := env.war.GetActiveWars()
	if len(wars) != 1 || !wars[0].AttackClans["ClanA"] || !wars[0].AttackClans["ClanC"] {
		t.Fatalf("第二攻方應加入同一場攻城戰，got %+v", wars)
	}
	// A 投降後 C 續戰
	env.war.SurrenderWar(a.Session, a, "ClanB")
	wars = env.war.GetActiveWars()
	if len(wars) != 1 || wars[0].AttackClans["ClanA"] || !wars[0].AttackClans["ClanC"] {
		t.Fatalf("A 投降後 C 應續戰，got %+v", wars)
	}
	// 時間到 → CeaseCastleWar 清掉
	env.war.CeaseCastleWar(1)
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("CeaseCastleWar 後應清空，got %d", n)
	}
}

// --- pending 請求綁定：錯 mode、換盟、換盟主、原戰已結束、覆蓋保護 ---

// 發 221 請求卻回 222 → 不生效，且請求仍保留。
func TestSimWarConsentWrongModeIgnored(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	env.war.SurrenderWar(a.Session, a, "ClanB")
	drainWarPackets(a)
	drainWarPackets(b)

	env.war.HandleWarConsent(b.Session, b, 222, true)
	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("錯誤 mode 不得結束戰爭，got %d", n)
	}
	env.war.HandleWarConsent(b.Session, b, 217, true)
	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("錯誤 mode 不得改動戰爭，got %d", n)
	}
	if env.war.pending[b.CharID] == nil {
		t.Fatal("錯誤 mode 不應消耗 pending")
	}

	// 正確 mode 仍可生效
	env.war.HandleWarConsent(b.Session, b, 221, true)
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("正確 mode 應結束戰爭，got %d", n)
	}
}

// 等待期間請求者換到另一個非零血盟（且成為其盟主）→ 回覆不得套用到新血盟。
func TestSimWarConsentRequesterSwitchedToOtherClan(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	// A 退出 ClanA 並建立 ClanC 當盟主
	env.addClan(3, "ClanC", a, 0)

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("換盟後的回覆不得成立戰爭，got %+v", env.war.GetActiveWars())
	}
	if env.war.IsClanInWar("ClanC") {
		t.Fatal("不得套用到新血盟")
	}
}

// 等待期間回應者換到另一個非零血盟 → 回覆不得生效。
func TestSimWarConsentResponderSwitchedToOtherClan(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	env.addClan(3, "ClanC", b, 0)

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("回應者換盟後不得成立戰爭，got %+v", env.war.GetActiveWars())
	}
}

// 等待期間請求者血盟換了盟主 → 回覆不得生效。
func TestSimWarConsentRequesterLostLeadership(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	newLeader := env.addPlayer(1010, "NewLeaderA", 1, 50)
	env.ws.Clans.GetClan(1).LeaderID = newLeader.CharID
	env.ws.Clans.GetClan(1).LeaderName = newLeader.Name

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("請求者失去盟主身分後不得成立戰爭，got %d", n)
	}
}

// 等待期間回應者血盟換了盟主 → 原回應者的回覆不得生效。
func TestSimWarConsentResponderLostLeadership(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	env.war.SurrenderWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	newLeader := env.addPlayer(1011, "NewLeaderB", 2, 50)
	env.ws.Clans.GetClan(2).LeaderID = newLeader.CharID

	env.war.HandleWarConsent(b.Session, b, 221, true)

	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("前盟主的回覆不得結束戰爭，got %d", n)
	}
}

// 原戰爭已結束又開新戰 → 舊投降請求的回覆不得套用到新戰爭。
func TestSimWarStaleSurrenderReplyAfterNewWar(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	oldWar := env.war.GetActiveWars()[0]
	env.war.SurrenderWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	// 原戰爭到期結束
	env.now = env.now.Add(241 * time.Minute)
	env.war.TickSimWar()
	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("原戰爭應已結束，got %d", n)
	}
	if env.war.pending[b.CharID] != nil {
		t.Fatal("原戰爭結束後綁定的請求應清除")
	}
	// 立刻開新戰
	env.war.startSimWar("ClanA", "ClanB")
	newWar := env.war.GetActiveWars()[0]
	if newWar == oldWar {
		t.Fatal("測試前提：新戰爭應為不同實例")
	}
	drainWarPackets(a)
	drainWarPackets(b)

	env.war.HandleWarConsent(b.Session, b, 221, true)

	if n := len(env.war.GetActiveWars()); n != 1 {
		t.Fatalf("過期投降回覆不得結束新戰爭，got %d", n)
	}
	if pk := drainWarPackets(a); len(pk) != 0 {
		t.Fatalf("不應送出任何封包，got %d", len(pk))
	}
}

// 原戰爭以休戰結束後 pending 立即清除（removeWar 路徑）。
func TestSimWarPendingClearedWhenWarRemoved(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.startSimWar("ClanA", "ClanB")
	env.war.SurrenderWar(a.Session, a, "ClanB")
	if env.war.pending[b.CharID] == nil {
		t.Fatal("測試前提：應有 pending")
	}
	env.war.endSimWar(env.war.GetActiveWars()[0], 3, "ClanA", "ClanB")
	if env.war.pending[b.CharID] != nil {
		t.Fatal("戰爭移除後 pending 應清除")
	}
}

// 同一接收者已有有效請求時，不同請求者／不同類型不得覆蓋。
func TestSimWarPendingNotOverwrittenByDifferentRequest(t *testing.T) {
	env, a, b := twoClans(t)
	c := env.addPlayer(1004, "LeaderC", 0, 50)
	env.addClan(3, "ClanC", c, 0)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)

	// C 也向 B 宣戰：B 已有 A 的有效請求 → 忽略，不送對話框
	env.war.DeclareWar(c.Session, c, "ClanB")
	if yn := yesNoTypes(drainWarPackets(b)); len(yn) != 0 {
		t.Fatalf("已有待回應請求時不應再送對話框，got %v", yn)
	}
	if req := env.war.pending[b.CharID]; req == nil || req.requesterCharID != a.CharID {
		t.Fatalf("pending 應仍為 A 的請求，got %+v", req)
	}

	// B 同意 → 成立 A-B，而非 C-B
	env.war.HandleWarConsent(b.Session, b, 217, true)
	if !env.war.IsWar("ClanA", "ClanB") || env.war.IsClanInWar("ClanC") {
		t.Fatalf("應成立 A-B 戰爭，got %+v", env.war.GetActiveWars())
	}
}

// 同一接收者的舊請求已失效（請求者離線）→ 新請求可取代。
func TestSimWarStalePendingReplacedByNewRequest(t *testing.T) {
	env, a, b := twoClans(t)
	c := env.addPlayer(1004, "LeaderC", 0, 50)
	env.addClan(3, "ClanC", c, 0)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	env.ws.RemovePlayer(a.SessionID)

	env.war.DeclareWar(c.Session, c, "ClanB")
	if yn := yesNoTypes(drainWarPackets(b)); !containsU16(yn, 217) {
		t.Fatalf("舊請求失效後新請求應送對話框，got %v", yn)
	}
	env.war.HandleWarConsent(b.Session, b, 217, true)
	if !env.war.IsWar("ClanC", "ClanB") {
		t.Fatalf("應成立 C-B 戰爭，got %+v", env.war.GetActiveWars())
	}
}

// 請求者重登（session 改變）後的回覆不得生效。
func TestSimWarConsentRequesterRelogged(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	env.ws.RemovePlayer(a.SessionID)
	a2 := env.addPlayer(a.CharID, a.Name, 1, 50)
	_ = a2

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if n := len(env.war.GetActiveWars()); n != 0 {
		t.Fatalf("請求者重登後舊請求不得生效，got %d", n)
	}
}

// TickSimWar 會清除失效 pending。
func TestSimWarTickPrunesStalePending(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	a.ClanID = 0

	env.war.TickSimWar()

	if env.war.pending[b.CharID] != nil {
		t.Fatal("失效 pending 應被 tick 清除")
	}
}

// 通用 TempID 被其他對話（如寵物改名）設定不影響戰爭回應。
func TestSimWarConsentIgnoresGenericTempID(t *testing.T) {
	env, a, b := twoClans(t)
	env.war.DeclareWar(a.Session, a, "ClanB")
	drainWarPackets(b)
	b.TempID = 99999 // 其他流程覆寫

	env.war.HandleWarConsent(b.Session, b, 217, true)

	if !env.war.IsWar("ClanA", "ClanB") {
		t.Fatal("戰爭回應不應依賴通用 TempID")
	}
}
