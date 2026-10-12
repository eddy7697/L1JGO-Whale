package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type CustomPointStore interface {
	AdminAdjust(context.Context, string, string, int64, string, string) (*persist.CustomPointBalance, error)
	Query(context.Context, string, string) (*persist.CustomPointQuery, error)
}

type gmPointAdjustment struct {
	Login, Currency, Memo string
	Delta                 int64
}

func parseGMPointAdjustment(args []string) (gmPointAdjustment, error) {
	request := gmPointAdjustment{Currency: "MP"}
	if len(args) < 3 {
		return request, persist.ErrCustomPointInput
	}
	request.Login = strings.ToLower(args[0])
	amountIndex := 1
	// A decimal-looking token must stay an amount even when it overflows int64.
	if strings.Trim(args[1], "+-0123456789") != "" {
		request.Currency, amountIndex = strings.ToUpper(args[1]), 2
	}
	if len(args) <= amountIndex+1 {
		return request, persist.ErrCustomPointInput
	}
	var err error
	request.Delta, err = strconv.ParseInt(args[amountIndex], 10, 64)
	if err != nil {
		return request, persist.ErrCustomPointInput
	}
	request.Memo = strings.TrimSpace(strings.Join(args[amountIndex+1:], " "))
	return request, persist.ValidateCustomPointAdjustment(request.Login, request.Currency, request.Delta, request.Memo)
}

func dispatchGMPoints(sess *net.Session, player *world.PlayerInfo, cmd string, args []string, deps *Deps) bool {
	if cmd != "cpadd" && cmd != "cpquery" {
		return false
	}
	// These account-wide operations remain administrator-only even when called directly.
	if player == nil || player.AccessLevel != world.AccessLevelGM {
		gmMsg(sess, "此指令需要 GM 權限。")
		return true
	}
	if deps == nil || deps.CustomPoints == nil {
		gmMsg(sess, "商城點數資料庫未初始化。")
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cmd == "cpadd" {
		request, err := parseGMPointAdjustment(args)
		if err != nil {
			gmMsg(sess, "用法：.cpadd <帳號> [幣別] <非零正負點數> <備註>（預設 MP；備註最多 255 字）")
			return true
		}
		balance, err := deps.CustomPoints.AdminAdjust(ctx, request.Login, request.Currency, request.Delta, request.Memo, player.Name)
		if err != nil {
			gmPointError(sess, deps, err)
			return true
		}
		gmMsgf(sess, "[%s] %s (%s) %+d，餘額：%s", request.Login, balance.DisplayName, balance.Currency, request.Delta, gmPointNumber(balance.Balance))
		if deps.Log != nil {
			deps.Log.Info("GM 商城點數調整", zap.String("gm", player.Name), zap.String("account", request.Login), zap.String("currency", request.Currency), zap.Int64("delta", request.Delta), zap.Int64("balance", balance.Balance), zap.String("memo", request.Memo))
		}
		return true
	}
	if len(args) < 1 || len(args) > 2 {
		gmMsg(sess, "用法：.cpquery <帳號> [幣別]")
		return true
	}
	login, currency := strings.ToLower(args[0]), ""
	if len(args) == 2 {
		currency = strings.ToUpper(args[1])
	}
	result, err := deps.CustomPoints.Query(ctx, login, currency)
	if err != nil {
		gmPointError(sess, deps, err)
		return true
	}
	if len(result.Balances) == 0 {
		gmMsgf(sess, "[%s] 無任何商城點數紀錄。", login)
	}
	for _, balance := range result.Balances {
		gmMsgf(sess, "[%s] %s (%s)：%s", login, balance.DisplayName, balance.Currency, gmPointNumber(balance.Balance))
	}
	if len(result.Logs) == 0 {
		gmMsg(sess, "最近無流水。")
	} else {
		gmMsgf(sess, "最近 %d 筆流水：", len(result.Logs))
	}
	for _, entry := range result.Logs {
		order := ""
		if entry.ExternalOrderID != "" {
			order = " order=" + entry.ExternalOrderID
		}
		gmMsgf(sess, "%s %s %+d bal=%s [%s/%s] GM=%s%s %s", entry.CreatedAt.In(time.Local).Format("01-02 15:04"), entry.Currency, entry.Delta, gmPointNumber(entry.BalanceAfter), entry.Type, entry.Source, entry.GMName, order, entry.Memo)
	}
	return true
}

func gmPointError(sess *net.Session, deps *Deps, err error) {
	for _, expected := range []error{persist.ErrCustomPointAccount, persist.ErrCustomPointCurrency, persist.ErrCustomPointInsufficient, persist.ErrCustomPointOverflow, persist.ErrCustomPointInput} {
		if errors.Is(err, expected) {
			gmMsgf(sess, "商城點數操作失敗：%s。", expected)
			return
		}
	}
	if deps.Log != nil {
		deps.Log.Error("GM 商城點數操作失敗", zap.Error(err))
	}
	gmMsg(sess, "商城點數資料庫操作失敗，請檢查伺服器日誌。")
}

func gmPointNumber(value int64) string {
	text := strconv.FormatInt(value, 10)
	start := 0
	if value < 0 {
		start = 1
	}
	for i := len(text) - 3; i > start; i -= 3 {
		text = fmt.Sprintf("%s,%s", text[:i], text[i:])
	}
	return text
}
