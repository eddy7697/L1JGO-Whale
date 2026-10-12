package handler

import (
	"testing"
	"time"

	"github.com/l1jgo/server/internal/world"
)

func TestBuffSnapshotUsesDeadlineWhenWorldTicksAreDelayed(t *testing.T) {
	p := &world.PlayerInfo{CharID: 1}
	p.AddBuff(&world.ActiveBuff{SkillID: 1000, TicksLeft: 1500, ExpiresAt: time.Now().Add(-time.Second), SetBraveSpeed: 1})
	p.AddBuff(&world.ActiveBuff{SkillID: 1001, TicksLeft: 1500, ExpiresAt: time.Now().Add(5 * time.Second), SetMoveSpeed: 1})
	rows := BuffRowsFromPlayer(p)
	if len(rows) != 1 || rows[0].SkillID != 1001 || rows[0].RemainingTime < 4 || rows[0].RemainingTime > 5 {
		t.Fatalf("saved stale or extended speed effects: %+v", rows)
	}
}
