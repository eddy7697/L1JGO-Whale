package system

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// This suite creates a unique schema and only synthetic accounts. It never
// resets the default schema or uses server.local.toml / production credentials.
func loginPostgresDB(t *testing.T) *persist.DB {
	t.Helper()
	dsn := os.Getenv("L1JGO_TEST_LOGIN_DSN")
	if dsn == "" {
		t.Skip("set L1JGO_TEST_LOGIN_DSN for isolated PostgreSQL login integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("codex_login_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 1 // the server-lifetime lease must not occupy the application pool
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := persist.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return &persist.DB{Pool: pool}
}

func TestLoginPostgresLifecycle(t *testing.T) {
	db := loginPostgresDB(t)
	ctx := context.Background()
	repo := persist.NewAccountRepo(db)
	chars := persist.NewCharacterRepo(db)
	const password = "synthetic-test-password"
	create := func(name string) {
		t.Helper()
		if _, err := repo.Create(ctx, name, password, "127.0.0.1", "test"); err != nil {
			t.Fatal(err)
		}
	}
	create("stale")
	character := &persist.CharacterRow{AccountName: "stale", Name: "synthetic_character", ClassType: 1, Level: 1, HP: 30, MaxHP: 30, MP: 10, MaxMP: 10, AC: 10, MapID: 2005}
	if err := chars.Create(ctx, character); err != nil {
		t.Fatal(err)
	}
	execSQL := func(sql string) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	isOnline := func(name string) bool {
		t.Helper()
		a, err := repo.Load(ctx, name)
		if err != nil || a == nil {
			t.Fatalf("load %s: %v", name, err)
		}
		return a.Online
	}
	execSQL("UPDATE accounts SET online=true; UPDATE characters SET online_status=true")
	lease, err := repo.BeginOnlineState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Close(context.Background()) })
	if isOnline("stale") {
		t.Fatal("startup left stale account online")
	}
	var characterOnline bool
	if err := db.Pool.QueryRow(ctx, "SELECT online_status FROM characters WHERE id=$1", character.ID).Scan(&characterOnline); err != nil || characterOnline {
		t.Fatalf("startup character flag=%v err=%v", characterOnline, err)
	}

	// A second core cannot clear the first core's live flags, even on another port.
	execSQL("UPDATE accounts SET online=true WHERE name='stale'")
	if other, err := repo.BeginOnlineState(ctx); err == nil {
		other.Close(ctx)
		t.Fatal("second server acquired online state")
	}
	if !isOnline("stale") {
		t.Fatal("failed lease reset another server's account")
	}
	if err := lease.Close(ctx); err != nil {
		t.Fatal(err)
	}
	lease, err = repo.BeginOnlineState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if isOnline("stale") {
		t.Fatal("restart did not recover stale flag")
	}

	ws := world.NewState()
	login := NewLoginSystem(repo, 30, zap.NewNop())
	now := time.Unix(100, 0)
	login.now = func() time.Time { return now }
	deps := &handler.Deps{AccountRepo: repo, CharRepo: chars, Login: login, World: ws, Log: zap.NewNop(), Config: &config.Config{Character: config.CharacterConfig{DefaultSlots: 6, AutoCreateAccounts: true}}}
	reg := packet.NewRegistry(zap.NewNop())
	handler.RegisterAll(reg, deps)
	store := l1net.NewSessionStore()
	input := NewInputSystem(&l1net.Server{}, reg, store, 32, repo, chars, persist.NewItemRepo(db), nil, ws, nil, nil, zap.NewNop())
	input.SetLoginSystem(login)
	var id uint64
	session := func() *l1net.Session { id++; s := newLoginTestSession(t, id); store.Add(s); return s }
	auth := func(s *l1net.Session, name, pass string, beanfun bool) byte {
		t.Helper()
		op := packet.C_OPCODE_LOGIN
		if beanfun {
			op = packet.C_OPCODE_SHIFT_SERVER
		}
		w := packet.NewWriterWithOpcode(op)
		if beanfun {
			w.WriteC(6)
		}
		w.WriteS(name)
		w.WriteS(pass)
		if err := reg.Dispatch(s, s.State(), w.Bytes()); err != nil {
			t.Fatal(err)
		}
		s.FlushOutput()
		result := byte(255)
		for len(s.OutQueue) > 0 {
			p := <-s.OutQueue
			if p[0] == packet.S_OPCODE_LOGIN_CHECK {
				result = byte(binary.LittleEndian.Uint16(p[1:3]))
			}
		}
		return result
	}
	first := session()
	if got := auth(first, "StAlE", password, false); got != 0 || first.AccountName != "stale" || first.State() != packet.StateAuthenticated || !isOnline("stale") {
		t.Fatalf("direct login result=%d state=%v", got, first.State())
	}

	// Both wire entrypoints reject duplicate authentication without displacing the owner.
	duplicate := session()
	if got := auth(duplicate, "stale", password, true); got != 7 || first.IsClosed() || login.owners["stale"] != first {
		t.Fatalf("duplicate result=%d", got)
	}
	if got := auth(session(), "stale", "wrong", false); got != 8 {
		t.Fatalf("wrong password=%d", got)
	}
	if got := auth(session(), "stale", "wrong", true); got != 149 {
		t.Fatalf("beanfun wrong password=%d", got)
	}
	if !isOnline("stale") || first.IsClosed() {
		t.Fatal("invalid login affected existing owner")
	}
	create("full")
	login.maxOnline = 1
	full := session()
	if got := auth(full, "full", password, false); got != 255 || !full.IsClosed() || isOnline("full") {
		t.Fatalf("server-full login=%d closed=%v", got, full.IsClosed())
	}
	create("monitor")
	execSQL("UPDATE accounts SET access_level=100 WHERE name='monitor'")
	monitor := session()
	if got := auth(monitor, "monitor", password, false); got != 0 {
		t.Fatalf("privileged login at capacity=%d", got)
	}
	monitor.Close()
	input.Update(0)
	login.maxOnline = 30
	create("banned")
	execSQL("UPDATE accounts SET banned=true WHERE name='banned'")
	if got := auth(session(), "banned", password, false); got != 8 || isOnline("banned") {
		t.Fatalf("banned login=%d", got)
	}
	if _, err := repo.TryMarkOnline(ctx, "missing"); err != persist.ErrAccountUnavailable {
		t.Fatalf("missing account claim=%v", err)
	}
	if _, err := repo.TryMarkOnline(ctx, "banned"); err != persist.ErrAccountUnavailable {
		t.Fatalf("banned account claim=%v", err)
	}

	// Simulate a socket EOF while still at character select; the next input poll saves/releases.
	first.Close()
	input.Update(0)
	if isOnline("stale") || login.owners["stale"] != nil {
		t.Fatal("selection-screen EOF leaked account")
	}
	if got := auth(duplicate, "stale", password, true); got != 0 {
		t.Fatalf("reconnect=%d", got)
	}
	login.Logout(first)
	if !isOnline("stale") || login.owners["stale"] != duplicate {
		t.Fatal("late old disconnect cleared new account")
	}

	// Failed DB admission must not send LOGIN_OK. Recovery includes ambiguous commit cleanup.
	create("reject_mark")
	execSQL(`CREATE FUNCTION reject_online() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name='reject_mark' AND NEW.online THEN RAISE EXCEPTION 'synthetic online failure'; END IF; RETURN NEW; END $$;
	CREATE TRIGGER reject_online BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION reject_online()`)
	rejected := session()
	if got := auth(rejected, "reject_mark", password, false); got != 8 || rejected.AccountName != "" || rejected.State() != packet.StateVersionOK {
		t.Fatalf("failed DB admission=%d", got)
	}
	execSQL("DROP TRIGGER reject_online ON accounts")
	input.Update(0)
	if got := auth(rejected, "reject_mark", password, false); got != 0 {
		t.Fatalf("DB recovery login=%d", got)
	}

	// Offline update failure is retried without freeing the account too early.
	execSQL(`CREATE FUNCTION reject_offline() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name='reject_mark' AND NOT NEW.online THEN RAISE EXCEPTION 'synthetic offline failure'; END IF; RETURN NEW; END $$;
	CREATE TRIGGER reject_offline BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION reject_offline()`)
	rejected.Close()
	input.Update(0)
	if !isOnline("reject_mark") || login.owners["reject_mark"] != rejected {
		t.Fatal("failed cleanup forgot the account")
	}
	execSQL("DROP TRIGGER reject_offline ON accounts")
	now = now.Add(time.Second)
	input.Update(0)
	if isOnline("reject_mark") || login.owners["reject_mark"] != nil {
		t.Fatal("offline recovery did not release account")
	}

	// A duplicate character must not overwrite world indexes even if its caller has stale auth state.
	active := &world.PlayerInfo{CharID: character.ID, SessionID: duplicate.ID, Session: duplicate, Name: character.Name}
	ws.AddPlayer(active)
	entrant := session()
	entrant.AccountName = "stale"
	entrant.SetState(packet.StateAuthenticated)
	w := packet.NewWriterWithOpcode(packet.C_OPCODE_ENTER_WORLD)
	w.WriteS(character.Name)
	if err := reg.Dispatch(entrant, entrant.State(), w.Bytes()); err != nil {
		t.Fatal(err)
	}
	if !entrant.IsClosed() || ws.GetByCharID(character.ID) != active {
		t.Fatal("second session replaced active character")
	}
	input.Update(0)
	if !isOnline("stale") {
		t.Fatal("rejected entrant cleared true owner's flag")
	}
	ws.RemovePlayer(duplicate.ID) // fixture represented world presence only, no gameplay inventory

	// Logout action is valid after authentication; no reauthentication is allowed.
	logout := packet.NewWriterWithOpcode(packet.C_OPCODE_SHIFT_SERVER)
	logout.WriteC(0x1c)
	if err := reg.Dispatch(duplicate, duplicate.State(), logout.Bytes()); err != nil {
		t.Fatal(err)
	}
	input.Update(0)
	if isOnline("stale") {
		t.Fatal("explicit logout left account online")
	}

	// Assert the real in-world character and inventory commits precede online=false.
	playing := session()
	if got := auth(playing, "stale", password, false); got != 0 {
		t.Fatal("world fixture login:", got)
	}
	playing.CharName = character.Name
	playing.SetState(packet.StateInWorld)
	inv := world.NewInventory()
	inv.AddItem(40308, 123, "synthetic adena", 0, 0, true, 1)
	ws.AddPlayer(&world.PlayerInfo{CharID: character.ID, SessionID: playing.ID, Session: playing, Name: character.Name,
		Level: 7, HP: 17, MaxHP: 45, MP: 9, MaxMP: 20, X: 24680, Y: 23456, MapID: 4, Inv: inv})
	execSQL(`CREATE FUNCTION verify_save_order() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
	IF NEW.name='stale' AND OLD.online AND NOT NEW.online THEN
	  IF NOT EXISTS(SELECT 1 FROM characters WHERE name='synthetic_character' AND level=7 AND hp=17 AND x=24680) OR
	     NOT EXISTS(SELECT 1 FROM character_items i JOIN characters c ON c.id=i.char_id WHERE c.name='synthetic_character' AND i.item_id=40308 AND i.count=123) THEN
	    RAISE EXCEPTION 'account released before character and inventory saves';
	  END IF;
	END IF; RETURN NEW; END $$;
	CREATE TRIGGER verify_save_order BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION verify_save_order()`)
	playing.Close()
	input.Update(0)
	if isOnline("stale") || login.owners["stale"] != nil || ws.GetByCharID(character.ID) != nil {
		t.Fatal("world disconnect did not save then release")
	}
	saved, err := chars.LoadByName(ctx, character.Name)
	if err != nil || saved.Level != 7 || saved.HP != 17 || saved.X != 24680 {
		t.Fatalf("character state not persisted: %v %v", saved, err)
	}
	var amount int
	if err := db.Pool.QueryRow(ctx, "SELECT count FROM character_items WHERE char_id=$1 AND item_id=40308", character.ID).Scan(&amount); err != nil || amount != 123 {
		t.Fatalf("inventory amount=%d err=%v", amount, err)
	}
	execSQL("DROP TRIGGER verify_save_order ON accounts")
	if err := input.DisconnectAll(); err != nil {
		t.Fatal(err)
	}
	if len(login.owners) != 0 {
		t.Fatal("shutdown leaked accounts")
	}
}
