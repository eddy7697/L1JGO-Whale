package handler

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
)

type gmPointTestStore struct {
	adjustments               []gmPointAdjustment
	gmName                    string
	queryLogin, queryCurrency string
	queries                   int
	err                       error
}

func (s *gmPointTestStore) AdminAdjust(_ context.Context, login, currency string, delta int64, memo, gmName string) (*persist.CustomPointBalance, error) {
	s.adjustments = append(s.adjustments, gmPointAdjustment{Login: login, Currency: currency, Delta: delta, Memo: memo})
	s.gmName = gmName
	return &persist.CustomPointBalance{Currency: currency, DisplayName: "商城點數", Balance: 12000}, s.err
}

func (s *gmPointTestStore) Query(_ context.Context, login, currency string) (*persist.CustomPointQuery, error) {
	s.queries++
	s.queryLogin, s.queryCurrency = login, currency
	return &persist.CustomPointQuery{
		Balances: []persist.CustomPointBalance{{Currency: "MP", DisplayName: "商城點數", Balance: 12000}},
		Logs:     []persist.CustomPointLog{{Currency: "MP", Delta: -300, BalanceAfter: 12000, Type: "admin_adjust", Source: "gm_command", Memo: "補償 更正", GMName: "Admin", CreatedAt: time.Now()}},
	}, s.err
}

func TestGMPointAdjustmentParsing(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		currency string
		delta    int64
		memo     string
	}{
		{[]string{"Alice", "1000", "活動", "補償"}, "MP", 1000, "活動 補償"},
		{[]string{"Alice", "mp", "-500", "更正"}, "MP", -500, "更正"},
		{[]string{"Alice", "9COIN", "5", "測試"}, "9COIN", 5, "測試"},
		{[]string{"Alice", "9223372036854775807", "上限"}, "MP", math.MaxInt64, "上限"},
	} {
		got, err := parseGMPointAdjustment(tc.args)
		if err != nil || got.Login != "alice" || got.Currency != tc.currency || got.Delta != tc.delta || got.Memo != tc.memo {
			t.Fatalf("args=%v result=%+v err=%v", tc.args, got, err)
		}
	}
	for _, args := range [][]string{
		{}, {"a", "1"}, {"a", "MP", "1"}, {"a", "0", "memo"},
		{"a", "9223372036854775808", "1", "memo"}, {"a", "-9223372036854775808", "memo"},
		{"a", "MP", "1.5", "memo"}, {"a", "1", " "}, {"a", "1", strings.Repeat("字", 256)}, {"a", "1", "nul\x00memo"},
	} {
		if _, err := parseGMPointAdjustment(args); err == nil {
			t.Errorf("invalid input accepted: %v", args)
		}
	}
}

func TestGMPointsRequireExactAdministrator(t *testing.T) {
	for _, level := range []int16{0, 100, 150, 201} {
		sess := newHandlerTestSession(t, 1)
		store := &gmPointTestStore{}
		for _, cmd := range []string{"cpadd", "cpquery"} {
			dispatchGMPoints(sess, &world.PlayerInfo{AccessLevel: level}, cmd, []string{"alice", "1", "memo"}, &Deps{CustomPoints: store})
		}
		if len(store.adjustments) != 0 || store.queries != 0 {
			t.Fatalf("level %d accessed custom points", level)
		}
	}
}

func TestGMPointsAdjustmentQueryAndFailureMessages(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	store := &gmPointTestStore{}
	deps := &Deps{CustomPoints: store}
	gm := &world.PlayerInfo{Name: "Admin", AccessLevel: world.AccessLevelGM, GMModeOff: true}
	dispatchGMPoints(sess, gm, "cpadd", []string{"Alice", "-300", "補償", "更正"}, deps)
	if len(store.adjustments) != 1 || store.adjustments[0].Memo != "補償 更正" || store.gmName != "Admin" {
		t.Fatalf("missing audit request: %+v", store)
	}
	messages := func() string {
		var all []string
		for _, payload := range drainHandlerTestPackets(sess) {
			reader := packet.NewReader(payload)
			reader.ReadC()
			all = append(all, reader.ReadS())
		}
		return strings.Join(all, "\n")
	}
	if got := messages(); !strings.Contains(got, "12,000") || !strings.Contains(got, "-300") {
		t.Fatalf("adjustment response=%s", got)
	}
	dispatchGMPoints(sess, gm, "cpquery", []string{"Alice", "mp"}, deps)
	if store.queryLogin != "alice" || store.queryCurrency != "MP" {
		t.Fatalf("query normalization: %+v", store)
	}
	if got := messages(); !strings.Contains(got, "admin_adjust/gm_command") || !strings.Contains(got, "GM=Admin") || !strings.Contains(got, "補償 更正") {
		t.Fatalf("audit response=%s", got)
	}
	store.err = persist.ErrCustomPointInsufficient
	dispatchGMPoints(sess, gm, "cpadd", []string{"Alice", "-99999", "memo"}, deps)
	if got := messages(); !strings.Contains(got, "餘額不足") || strings.Contains(got, "12,000") {
		t.Fatalf("failed adjustment claimed success: %s", got)
	}
	store.err = errors.New("private database detail")
	dispatchGMPoints(sess, gm, "cpquery", []string{"Alice"}, deps)
	if got := messages(); strings.Contains(got, "private database detail") || !strings.Contains(got, "操作失敗") {
		t.Fatalf("database failure response=%s", got)
	}
}
