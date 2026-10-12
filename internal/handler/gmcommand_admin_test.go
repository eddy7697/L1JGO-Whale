package handler

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func TestGMShutdownCountdownAbortAndOneShot(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	deps := &Deps{World: world.NewState(), GMAdmin: NewGMAdminState()}
	calls := 0
	deps.GMAdmin.Shutdown = func() { calls++ }
	before := time.Now()
	gmAdminShutdown(sess, []string{"1"}, deps)
	deadline := deps.GMAdmin.ShutdownAt
	if deadline.Before(before.Add(5*time.Second)) || deadline.After(time.Now().Add(5*time.Second)) {
		t.Fatal("Java 關機最短倒數應為 5 秒")
	}
	GMAdminTick(deadline.Add(-time.Millisecond), deps)
	if calls != 0 {
		t.Fatal("倒數未到期不得關機")
	}
	gmAdminShutdown(sess, []string{"abort"}, deps)
	GMAdminTick(deadline.Add(time.Second), deps)
	if calls != 0 {
		t.Fatal("abort 後不得再觸發先前倒數")
	}
	gmAdminShutdown(sess, []string{"60"}, deps)
	GMAdminTick(deps.GMAdmin.ShutdownAt, deps)
	GMAdminTick(time.Now().Add(time.Hour), deps)
	if calls != 1 {
		t.Fatalf("關機回呼應僅呼叫一次，got=%d", calls)
	}
}

func TestGMChatMutePersistenceAndRelease(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	targetSess := newHandlerTestSession(t, 2)
	deps := &Deps{World: world.NewState(), GMAdmin: NewGMAdminState()}
	target := &world.PlayerInfo{CharID: 12, Name: "Target", SessionID: 2, Session: targetSess}
	deps.World.AddPlayer(target)
	gmAdminChatNG(sess, []string{"target", "10"}, deps)
	if !IsGMChatProhibited(target, deps) {
		t.Fatal("禁言狀態未套用")
	}
	rows := BuffRowsFromPlayer(target)
	if len(rows) != 1 || rows[0].SkillID != 1005 || rows[0].RemainingTime != 600 {
		t.Fatalf("禁言必須持久化 600 秒: %+v", rows)
	}
	gmAdminChatNG(sess, []string{"Target", "-1"}, deps)
	if !IsGMChatProhibited(target, deps) {
		t.Fatal("非法時間不能解除既有禁言")
	}
	gmAdminChatNG(sess, []string{"Target", "0"}, deps)
	if IsGMChatProhibited(target, deps) || !target.Dirty {
		t.Fatal("解除禁言必須刪除 buff 並標示存檔")
	}
}

func TestGMSKickMovesAndClosesWithoutRemovingPlayer(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	targetSess := newHandlerTestSession(t, 2)
	deps := &Deps{World: world.NewState()}
	target := &world.PlayerInfo{CharID: 12, Name: "Target", SessionID: 2, Session: targetSess, X: 100, Y: 100, MapID: 7, ShowID: 99}
	deps.World.AddPlayer(target)
	gmAdminKick(sess, "skick", []string{"Target"}, deps)
	if !targetSess.IsClosed() || target.X != 33080 || target.Y != 33392 || target.MapID != 4 || target.ShowID != 0 {
		t.Fatalf("SKick 未送往銀騎士村後斷線: %+v", target)
	}
	if deps.World.GetBySession(2) != target {
		t.Fatal("踢除應保留世界玩家供 InputSystem 走完整登出存檔")
	}
	pkt := <-targetSess.OutQueue
	if pkt[0] != packet.S_OPCODE_KICK || binary.LittleEndian.Uint16(pkt[1:3]) != 500 {
		t.Fatalf("錯誤斷線封包: %v", pkt)
	}
}

func TestGMCleaningRetainsOnlyOccupiedTileInSameInstance(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	ws := world.NewState()
	p := &world.PlayerInfo{CharID: 1, SessionID: 1, Session: sess, X: 100, Y: 100, MapID: 4, Known: world.NewKnownEntities()}
	ws.AddPlayer(p)
	for _, item := range []*world.GroundItem{
		{ID: 11, ItemID: 40308, X: 100, Y: 100, MapID: 4},
		{ID: 12, ItemID: 40308, X: 101, Y: 100, MapID: 4},
		{ID: 13, ItemID: 40308, X: 100, Y: 100, MapID: 4, ShowID: 1},
	} {
		ws.AddGroundItem(item)
		p.Known.GroundItems[item.ID] = world.KnownPos{X: item.X, Y: item.Y}
	}
	gmAdminCleaning(sess, &Deps{World: ws})
	if ws.GetGroundItem(11) == nil || ws.GetGroundItem(12) != nil || ws.GetGroundItem(13) != nil {
		t.Fatal("清除必須保留同副本玩家腳下物品；鄰格與其他副本不受此保護")
	}
	if _, found := p.Known.GroundItems[12]; found {
		t.Fatal("已刪地面物仍留在玩家可見物件表")
	}
}

func TestGMSaveReportsFailureInsteadOfSuccess(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	deps := &Deps{World: world.NewState(), GMAdmin: NewGMAdminState(), Log: zap.NewNop()}
	calls := 0
	deps.GMAdmin.SaveAll = func() error { calls++; return errors.New("database offline") }
	gmAdminSave(sess, deps)
	packets := drainHandlerTestPackets(sess)
	if calls != 1 || len(packets) != 1 {
		t.Fatalf("存檔失敗應回覆一個訊息 calls=%d packets=%d", calls, len(packets))
	}
	r := packet.NewReader(packets[0])
	r.ReadC()
	message := r.ReadS()
	if strings.Contains(message, "已儲存") {
		t.Fatalf("失敗不得宣告已存檔: %s", message)
	}
}

func TestGMGiftUsesJavaArgumentOrderAndTemplateState(t *testing.T) {
	items := mustLoadGMItemCommandItems(t)
	request, err := parseGMGiftRequest("present", []string{"Account", "31", "7", "3"}, items)
	if err != nil {
		t.Fatal(err)
	}
	if request.Account != "account" || request.ItemID != 31 || request.Enchant != 7 || request.Count != 3 || request.Stackable {
		t.Fatalf("武器倉庫贈品解析錯誤: %+v", request)
	}
	request, err = parseGMGiftRequest("lvpresent", []string{"45", "80", "40010", "0", "50"}, items)
	if err != nil {
		t.Fatal(err)
	}
	if !request.ByLevel || request.MinLevel != 45 || request.MaxLevel != 80 || !request.Stackable || request.Count != 50 {
		t.Fatalf("等級限定堆疊物品解析錯誤: %+v", request)
	}
	for _, args := range [][]string{{"*", "31", "128", "1"}, {"*", "31", "-128", "1"}, {"*", "31", "0", "0"}, {"*", "31", "0", "2147483648"}, {"*", "99999999", "0", "1"}, {"*", "31", "0"}} {
		if _, err := parseGMGiftRequest("present", args, items); err == nil {
			t.Errorf("非法贈品參數未拒絕: %v", args)
		}
	}
	if _, err := parseGMGiftRequest("lvpresent", []string{"80", "45", "31", "0", "1"}, items); err == nil {
		t.Fatal("倒置等級範圍未拒絕")
	}
}
