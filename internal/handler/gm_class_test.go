package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/l1jgo/server/internal/scripting"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type captureGMClassStore struct {
	name      string
	classType int16
	gfx       int32
	err       error
}

func (s *captureGMClassStore) SetClass(_ context.Context, name string, classType int16, gfx int32) error {
	s.name, s.classType, s.gfx = name, classType, gfx
	return s.err
}

func TestGMClassUsesPersistedSexAndCreationSprites(t *testing.T) {
	engine, err := scripting.NewEngine("../../scripts", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	deps := &Deps{Scripting: engine}
	for _, tc := range []struct {
		sex, classType int16
		gfx            int32
	}{
		{0, 0, 0}, {1, 0, 1}, {0, 1, 61}, {1, 1, 48}, {0, 2, 138}, {1, 2, 37},
		{0, 3, 734}, {1, 3, 1186}, {0, 4, 2786}, {1, 4, 2796},
		{0, 5, 6658}, {1, 5, 6661}, {0, 6, 6671}, {1, 6, 6650},
	} {
		p := &world.PlayerInfo{Name: "test", ClassType: 2, ClassID: 138, Sex: tc.sex, TempCharGfx: 5641}
		store := &captureGMClassStore{}
		if err := changeGMClass(p, tc.classType, deps, store); err != nil {
			t.Fatal(err)
		}
		if p.ClassType != tc.classType || p.ClassID != tc.gfx || p.Sex != tc.sex || !p.Dirty {
			t.Fatalf("職業 %d 性別 %d 的角色狀態錯誤：%+v", tc.classType, tc.sex, p)
		}
		if store.name != p.Name || store.classType != tc.classType || store.gfx != tc.gfx {
			t.Fatalf("職業與外觀必須一併持久化：%+v", store)
		}
		if p.TempCharGfx != 5641 {
			t.Fatal("改職業不應清除既有暫時變身")
		}
	}
}

func TestGMClassPersistenceFailureKeepsRuntimeIdentity(t *testing.T) {
	engine, err := scripting.NewEngine("../../scripts", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	p := &world.PlayerInfo{Name: "test", ClassType: 2, ClassID: 138, Sex: 0}
	store := &captureGMClassStore{err: errors.New("database unavailable")}
	if err := changeGMClass(p, 1, &Deps{Scripting: engine}, store); err == nil {
		t.Fatal("必須回報存檔失敗")
	}
	if p.ClassType != 2 || p.ClassID != 138 || p.Dirty {
		t.Fatal("存檔失敗不應先更動線上職業")
	}
}
