package system

import (
	"context"
	"errors"
	"fmt"
	"time"

	coresys "github.com/l1jgo/server/internal/core/system"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// PersistenceSystem periodically auto-saves all online players' character data,
// inventory, bookmarks, known spells, and active buffs. Phase 5 (Persist).
type PersistenceSystem struct {
	world     *world.State
	charRepo  *persist.CharacterRepo
	itemRepo  *persist.ItemRepo
	buffRepo  *persist.BuffRepo
	walRepo   *persist.WALRepo
	log       *zap.Logger
	tickCount int
	interval  int // auto-save every N ticks
}

func NewPersistenceSystem(ws *world.State, charRepo *persist.CharacterRepo, itemRepo *persist.ItemRepo, buffRepo *persist.BuffRepo, walRepo *persist.WALRepo, log *zap.Logger, intervalTicks int) *PersistenceSystem {
	return &PersistenceSystem{
		world:    ws,
		charRepo: charRepo,
		itemRepo: itemRepo,
		buffRepo: buffRepo,
		walRepo:  walRepo,
		log:      log,
		interval: intervalTicks,
	}
}

func (s *PersistenceSystem) Phase() coresys.Phase { return coresys.PhasePersist }

func (s *PersistenceSystem) Update(_ time.Duration) {
	s.tickCount++
	if s.tickCount < s.interval {
		return
	}
	s.tickCount = 0
	s.saveAllPlayers()
}

// SaveAllPlayers persists all online players immediately, ignoring dirty flags.
// Called for graceful shutdown to ensure no data is lost.
func (s *PersistenceSystem) SaveAllPlayers() {
	s.savePlayers(false) // dirtyOnly=false → save all for shutdown safety
}

// SaveAllPlayersWithError 供 GM 存檔和關機使用，任一步失敗都回傳錯誤，避免顯示成功。
func (s *PersistenceSystem) SaveAllPlayersWithError() error {
	return s.savePlayers(false)
}

func (s *PersistenceSystem) saveAllPlayers() {
	s.savePlayers(true) // dirtyOnly=true → only save players with dirty flag
}

// savePlayers persists player data. If dirtyOnly is true, only saves players
// whose Dirty flag is set and resets the flag after successful save.
func (s *PersistenceSystem) savePlayers(dirtyOnly bool) error {
	count := 0
	var saveErr error
	s.world.AllPlayers(func(p *world.PlayerInfo) {
		if dirtyOnly && !p.Dirty {
			return // skip clean players — no state change since last save
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var playerErr error
		recordError := func(step string, err error) {
			if err == nil {
				return
			}
			s.log.Error(step, zap.String("name", p.Name), zap.Error(err))
			playerErr = errors.Join(playerErr, fmt.Errorf("%s (%s): %w", step, p.Name, err))
		}

		// 儲存時必須扣除裝備加成和 buff 加成，只保存基礎值。
		// 否則重新登入時 InitEquipStats / loadAndRestoreBuffs 會重複疊加，造成屬性膨脹。
		eq := p.EquipBonuses
		var bStr, bDex, bCon, bWis, bIntel, bCha int16
		var bMaxHP, bMaxMP int32
		for _, b := range p.ActiveBuffs {
			bStr += b.DeltaStr
			bDex += b.DeltaDex
			bCon += b.DeltaCon
			bWis += b.DeltaWis
			bIntel += b.DeltaIntel
			bCha += b.DeltaCha
			bMaxHP += b.DeltaMaxHP
			bMaxMP += b.DeltaMaxMP
		}
		row := &persist.CharacterRow{
			Name:        p.Name,
			OriginalStr: p.OriginalStr,
			OriginalCon: p.OriginalCon,
			Level:       p.Level,
			Exp:         int64(p.Exp),
			HP:          p.HP,
			MP:          p.MP,
			MaxHP:       p.MaxHP - int32(eq.AddHP) - bMaxHP,
			MaxMP:       p.MaxMP - int32(eq.AddMP) - bMaxMP,
			X:           p.X,
			Y:           p.Y,
			MapID:       p.MapID,
			Heading:     p.Heading,
			Lawful:      p.Lawful,
			Str:         p.Str - int16(eq.AddStr) - bStr,
			Dex:         p.Dex - int16(eq.AddDex) - bDex,
			Con:         p.Con - int16(eq.AddCon) - bCon,
			Wis:         p.Wis - int16(eq.AddWis) - bWis,
			Cha:         p.Cha - int16(eq.AddCha) - bCha,
			Intel:       p.Intel - int16(eq.AddInt) - bIntel,
			BonusStats:  p.BonusStats,
			ElixirStats: p.ElixirStats,
			ClanID:      p.ClanID,
			ClanName:    p.ClanName,
			ClanRank:    p.ClanRank,
			Title:       p.Title,
			Karma:       p.Karma,
			PKCount:     p.PKCount,
			HellTime:    p.HellTime,
			Food:        p.Food,
		}
		recordError("自動存檔角色失敗", s.charRepo.SaveCharacter(ctx, row))
		recordError("自動存檔背包失敗", s.itemRepo.SaveInventory(ctx, p.CharID, p.Inv, &p.Equip))
		recordError("自動存檔書籤失敗", s.charRepo.SaveBookmarks(ctx, p.Name, bookmarksToRows(p.Bookmarks)))
		recordError("自動存檔魔法書失敗", s.charRepo.SaveKnownSpells(ctx, p.Name, p.KnownSpells))
		if len(p.MapTimeUsed) > 0 {
			recordError("自動存檔限時地圖時間失敗", s.charRepo.SaveMapTimes(ctx, p.Name, p.MapTimeUsed))
		}
		// Save active buffs (including polymorph state)
		if s.buffRepo != nil {
			buffRows := handler.BuffRowsFromPlayer(p)
			recordError("自動存檔buff失敗", s.buffRepo.SaveBuffs(ctx, p.CharID, buffRows))
		}
		if playerErr != nil {
			p.Dirty = true
			saveErr = errors.Join(saveErr, playerErr)
			return
		}
		p.Dirty = false
		count++
	})
	if count > 0 {
		s.log.Info("自動存檔完成", zap.Int("玩家數", count))
	}

	// Mark WAL entries as processed after successful batch save.
	// This prevents replay of already-persisted economic transactions on crash recovery.
	if s.walRepo != nil && saveErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.walRepo.MarkProcessed(ctx); err != nil {
			s.log.Error("WAL MarkProcessed 失敗", zap.Error(err))
			saveErr = errors.Join(saveErr, err)
		}
	}
	return saveErr
}

// bookmarksToRows is defined in input.go (shared within the system package).
