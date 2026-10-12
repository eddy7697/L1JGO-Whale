package handler

import (
	"context"
	"time"

	"github.com/l1jgo/server/internal/net"
	"go.uber.org/zap"
)

func gmCPShopReload(sess *net.Session, deps *Deps) {
	if deps.CPShops == nil {
		gmMsg(sess, "商城點數商店資料尚未初始化。")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shops, items, err := deps.CPShops.Reload(ctx)
	if err != nil {
		if deps.Log != nil {
			deps.Log.Error("商城點數資料重載失敗", zap.Error(err))
		}
		gmMsg(sess, "商城點數熱重載失敗，舊快取保留；請查 server log。")
		return
	}
	gmMsgf(sess, "商城點數熱重載完成：商店 %d 家 / 啟用商品 %d 筆。", shops, items)
}
