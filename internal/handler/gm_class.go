package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/l1jgo/server/internal/world"
)

type gmClassStore interface {
	SetClass(context.Context, string, int16, int32) error
}

// changeGMClass 與創角共用 Lua 職業外觀，先持久化再切換執行期狀態。
func changeGMClass(player *world.PlayerInfo, classType int16, deps *Deps, store gmClassStore) error {
	if classType < 0 || classType > 6 || (player.Sex != 0 && player.Sex != 1) {
		return fmt.Errorf("無效的職業或角色性別")
	}
	if deps.Scripting == nil || store == nil {
		return fmt.Errorf("角色資料庫或職業資料未初始化")
	}
	info := deps.Scripting.GetCharCreateData(int(classType))
	if info == nil {
		return fmt.Errorf("找不到職業資料")
	}
	gfx := int32(info.MaleGFX)
	if player.Sex == 1 {
		gfx = int32(info.FemaleGFX)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.SetClass(ctx, player.Name, classType, gfx); err != nil {
		return err
	}
	player.ClassType, player.ClassID = classType, gfx
	player.Dirty = true
	return nil
}
