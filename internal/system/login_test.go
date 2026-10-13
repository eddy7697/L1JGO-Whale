package system

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type loginOnlineFake struct {
	online               map[string]bool
	claimErr, offlineErr error
	claims, releases     int
	afterClaim           func()
	afterCommitErr       error
}

func (f *loginOnlineFake) TryMarkOnline(_ context.Context, name string) (bool, error) {
	f.claims++
	if f.claimErr != nil {
		return false, f.claimErr
	}
	if f.online[name] {
		return false, nil
	}
	f.online[name] = true
	if f.afterClaim != nil {
		f.afterClaim()
	}
	return true, f.afterCommitErr
}

func (f *loginOnlineFake) SetOnline(_ context.Context, name string, online bool) error {
	f.releases++
	if f.offlineErr != nil {
		return f.offlineErr
	}
	f.online[name] = online
	return nil
}

func newLoginTestSession(t *testing.T, id uint64) *l1net.Session {
	sess := newSkillTestSession(t, id)
	sess.IP = "127.0.0.1:12345"
	sess.SetState(packet.StateVersionOK)
	return sess
}

func TestLoginNormalDuplicateAndAtomicClaim(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}}
	s := NewLoginSystem(f, 30, zap.NewNop())
	old := newLoginTestSession(t, 1)
	account := &persist.AccountRow{Name: "account"}
	if err := s.Login(context.Background(), old, account); err != nil {
		t.Fatal(err)
	}
	for _, dbOnline := range []bool{true, false} {
		account.Online = dbOnline
		if !dbOnline {
			f.online[account.Name] = true
			s.owners = make(map[string]*l1net.Session)
		}
		other := newLoginTestSession(t, 2)
		if err := s.Login(context.Background(), other, account); !errors.Is(err, handler.ErrAccountOnline) {
			t.Fatalf("duplicate: %v", err)
		}
		if old.IsClosed() || old.State() == packet.StateDisconnecting || other.AccountName != "" {
			t.Fatal("ordinary duplicate displaced an owner")
		}
	}
}

func TestLoginLimitCountsCharacterSelectAndPrivilege(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}}
	s := NewLoginSystem(f, 1, zap.NewNop())
	first := newLoginTestSession(t, 1)
	if err := s.Login(context.Background(), first, &persist.AccountRow{Name: "first"}); err != nil {
		t.Fatal(err)
	}
	first.SetState(packet.StateAuthenticated)
	if err := s.Login(context.Background(), newLoginTestSession(t, 2), &persist.AccountRow{Name: "second"}); !errors.Is(err, handler.ErrServerFull) {
		t.Fatalf("full: %v", err)
	}
	for i, access := range []int16{1, 100, 200} {
		name := []string{"privileged", "monitor", "gm"}[i]
		if err := s.Login(context.Background(), newLoginTestSession(t, uint64(i+3)), &persist.AccountRow{Name: name, AccessLevel: access}); err != nil {
			t.Fatalf("privilege %d: %v", access, err)
		}
	}
}

func TestLoginClaimFailureAndClosedDuringClaim(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}, claimErr: errors.New("database unavailable")}
	s := NewLoginSystem(f, 30, zap.NewNop())
	sess := newLoginTestSession(t, 1)
	account := &persist.AccountRow{Name: "account"}
	if err := s.Login(context.Background(), sess, account); err == nil {
		t.Fatal("DB failure accepted")
	}
	if sess.AccountName != "" {
		t.Fatal("failed claim authenticated a session")
	}
	s.Tick() // resolve a possibly committed UPDATE before allowing a retry
	f.claimErr = nil
	f.afterClaim = sess.Close
	if err := s.Login(context.Background(), sess, account); err != nil {
		t.Fatal(err)
	}
	s.Logout(sess)
	if f.online[account.Name] || len(s.owners) != 0 {
		t.Fatal("socket close during claim leaked online state")
	}
	before := f.claims
	if err := s.Login(context.Background(), sess, account); err == nil || f.claims != before {
		t.Fatal("closed session reauthenticated")
	}
}

func TestLoginOfflineRetryAndLateOldDisconnect(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}}
	s := NewLoginSystem(f, 30, zap.NewNop())
	now := time.Unix(100, 0)
	s.now = func() time.Time { return now }
	old := newLoginTestSession(t, 1)
	account := &persist.AccountRow{Name: "account"}
	if err := s.Login(context.Background(), old, account); err != nil {
		t.Fatal(err)
	}
	old.Close()
	f.offlineErr = errors.New("offline write failed")
	s.Logout(old)
	if s.owners[account.Name] != old || !f.online[account.Name] {
		t.Fatal("failed cleanup released owner")
	}
	s.Tick()
	if f.releases != 1 {
		t.Fatal("offline retry busy loop")
	}
	if err := s.Login(context.Background(), newLoginTestSession(t, 2), account); !errors.Is(err, handler.ErrAccountOnline) {
		t.Fatalf("cleanup must block replacement: %v", err)
	}
	f.offlineErr = nil
	now = now.Add(time.Second)
	s.Tick()
	fresh := newLoginTestSession(t, 3)
	if err := s.Login(context.Background(), fresh, account); err != nil {
		t.Fatal(err)
	}
	releases := f.releases
	s.Logout(old)
	if f.releases != releases || !f.online[account.Name] || s.owners[account.Name] != fresh {
		t.Fatal("late old cleanup cleared new owner")
	}
}

func TestLoginJavaSecondaryCollisionWaitsForCleanup(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}}
	s := NewLoginSystem(f, 30, zap.NewNop())
	now := time.Unix(100, 0)
	s.now = func() time.Time { return now }
	old := newLoginTestSession(t, 1)
	account := &persist.AccountRow{Name: "account"}
	if err := s.Login(context.Background(), old, account); err != nil {
		t.Fatal(err)
	}
	old.CharName = "synthetic"
	old.SetState(packet.StateInWorld)
	f.online[account.Name] = false // simulate the Java DB/map inconsistency path
	fresh := newLoginTestSession(t, 2)
	if err := s.Login(context.Background(), fresh, account); !errors.Is(err, handler.ErrLoginCollision) {
		t.Fatalf("collision: %v", err)
	}
	select {
	case p := <-old.OutQueue:
		if len(p) < 3 || p[0] != 71 || binary.LittleEndian.Uint16(p[1:3]) != 357 {
			t.Fatalf("want message 357, got %v", p)
		}
	default:
		t.Fatal("missing duplicate-login notice")
	}
	if fresh.AccountName != "" || old.IsClosed() || old.State() != packet.StateDisconnecting {
		t.Fatal("collision allowed replacement or skipped notice delay")
	}
	now = now.Add(time.Second)
	s.Tick()
	if !old.IsClosed() || s.owners[account.Name] != old {
		t.Fatal("must keep owner through character save")
	}
	s.Logout(old)
	if err := s.Login(context.Background(), fresh, account); err != nil {
		t.Fatal(err)
	}
}

func TestLoginDisconnectAtCharacterSelectAndShutdown(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}}
	s := NewLoginSystem(f, 30, zap.NewNop())
	store := l1net.NewSessionStore()
	reg := packet.NewRegistry(zap.NewNop())
	input := NewInputSystem(&l1net.Server{}, reg, store, 32, nil, nil, nil, nil, world.NewState(), nil, nil, zap.NewNop())
	input.SetLoginSystem(s)
	for i, name := range []string{"eof", "shutdown"} {
		sess := newLoginTestSession(t, uint64(i+1))
		if err := s.Login(context.Background(), sess, &persist.AccountRow{Name: name}); err != nil {
			t.Fatal(err)
		}
		sess.SetState(packet.StateAuthenticated)
		store.Add(sess)
	}
	store.Get(1).Close()
	input.Update(0)
	if f.online["eof"] || !f.online["shutdown"] {
		t.Fatal("character select disconnect incorrectly released accounts")
	}
	if err := input.DisconnectAll(); err != nil {
		t.Fatal(err)
	}
	if len(s.owners) != 0 || f.online["shutdown"] || len(store.Raw()) != 0 {
		t.Fatal("shutdown left authenticated account online")
	}
}

func TestLoginAmbiguousCommitRecoveryDoesNotClearOtherAccount(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}, afterCommitErr: context.DeadlineExceeded}
	s := NewLoginSystem(f, 30, zap.NewNop())
	sess := newLoginTestSession(t, 1)
	if err := s.Login(context.Background(), sess, &persist.AccountRow{Name: "ambiguous"}); err == nil {
		t.Fatal("timeout reported success")
	}
	if !f.online["ambiguous"] || sess.AccountName != "" {
		t.Fatal("fixture did not simulate commit without acknowledgement")
	}
	f.afterCommitErr = nil
	if err := s.Login(context.Background(), sess, &persist.AccountRow{Name: "another"}); err != nil {
		t.Fatal(err)
	}
	s.Tick()
	if f.online["ambiguous"] || !f.online["another"] || s.owners["another"] != sess {
		t.Fatal("failed-claim cleanup used the session's new account name")
	}
	if err := s.Login(context.Background(), newLoginTestSession(t, 2), &persist.AccountRow{Name: "ambiguous"}); err != nil {
		t.Fatal("account stranded after ambiguous commit:", err)
	}
}

func TestLoginUnavailableAccountDoesNotQueueOfflineWrite(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{"removed": true}, claimErr: persist.ErrAccountUnavailable}
	s := NewLoginSystem(f, 30, zap.NewNop())
	if err := s.Login(context.Background(), newLoginTestSession(t, 1), &persist.AccountRow{Name: "removed"}); !errors.Is(err, persist.ErrAccountUnavailable) {
		t.Fatal(err)
	}
	s.Tick()
	if len(s.owners) != 0 || f.releases != 0 || !f.online["removed"] {
		t.Fatal("unavailable account caused an unrelated offline write")
	}
}

func TestLoginPendingCleanupDoesNotConsumeOnlineCapacity(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}}
	s := NewLoginSystem(f, 1, zap.NewNop())
	old := newLoginTestSession(t, 1)
	if err := s.Login(context.Background(), old, &persist.AccountRow{Name: "old"}); err != nil {
		t.Fatal(err)
	}
	old.Close()
	f.offlineErr = errors.New("synthetic cleanup failure")
	s.Logout(old)
	if err := s.Login(context.Background(), newLoginTestSession(t, 2), &persist.AccountRow{Name: "different"}); err != nil {
		t.Fatal("unrelated account blocked by cleanup:", err)
	}
	if s.owners["old"] != old {
		t.Fatal("lost unfinished cleanup")
	}
}

func TestLoginClosedOwnerCleanedBeforeQueuedReconnect(t *testing.T) {
	f := &loginOnlineFake{online: map[string]bool{}}
	s := NewLoginSystem(f, 30, zap.NewNop())
	store := l1net.NewSessionStore()
	old, fresh := newLoginTestSession(t, 1), newLoginTestSession(t, 2)
	if err := s.Login(context.Background(), old, &persist.AccountRow{Name: "account"}); err != nil {
		t.Fatal(err)
	}
	store.Add(old)
	store.Add(fresh)
	reg := packet.NewRegistry(zap.NewNop())
	var loginErr error
	reg.Register(packet.C_OPCODE_LOGIN, []packet.SessionState{packet.StateVersionOK}, func(_ any, _ *packet.Reader) {
		loginErr = s.Login(context.Background(), fresh, &persist.AccountRow{Name: "account", Online: f.online["account"]})
	})
	input := NewInputSystem(&l1net.Server{}, reg, store, 32, nil, nil, nil, nil, world.NewState(), nil, nil, zap.NewNop())
	input.SetLoginSystem(s)
	old.Close()
	fresh.InQueue <- []byte{packet.C_OPCODE_LOGIN}
	input.Update(0)
	if loginErr != nil || s.owners["account"] != fresh {
		t.Fatal("closed owner was not cleaned before reconnect:", loginErr)
	}
}
