package system

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/persist"
	"go.uber.org/zap"
)

type accountOnlineStore interface {
	TryMarkOnline(context.Context, string) (bool, error)
	SetOnline(context.Context, string, bool) error
}

// LoginSystem owns authenticated accounts, including sessions at character select.
// All methods run on the game loop; network goroutines only close the transport.
type LoginSystem struct {
	repo      accountOnlineStore
	maxOnline int
	owners    map[string]*net.Session
	retryAt   map[string]time.Time
	kickAt    map[*net.Session]time.Time
	now       func() time.Time
	log       *zap.Logger
}

func NewLoginSystem(repo accountOnlineStore, maxOnline int, log *zap.Logger) *LoginSystem {
	return &LoginSystem{repo: repo, maxOnline: maxOnline, owners: make(map[string]*net.Session),
		retryAt: make(map[string]time.Time), kickAt: make(map[*net.Session]time.Time), now: time.Now, log: log}
}

func (s *LoginSystem) Login(ctx context.Context, sess *net.Session, account *persist.AccountRow) error {
	if sess.IsClosed() || sess.State() != packet.StateVersionOK || sess.AccountName != "" {
		return fmt.Errorf("session is not awaiting authentication")
	}
	// C_AuthLogin checks the persisted flag BEFORE LoginController. Ordinary
	// duplicate login returns 7 and does not kick the existing client.
	if account.Online {
		return handler.ErrAccountOnline
	}
	// Java Account.isGameMaster is access_level > 0 for admission, including monitors.
	if s.onlineCount() >= s.maxOnline && account.AccessLevel <= 0 {
		return handler.ErrServerFull
	}
	if old := s.owners[account.Name]; old != nil {
		// A closed owner may still be saving or retrying an offline update. Do not
		// permit a replacement until that cleanup completes.
		if old.IsClosed() || !s.retryAt[account.Name].IsZero() {
			return handler.ErrAccountOnline
		}
		// Java's secondary collision path kicks BOTH connections; it is not a
		// seamless takeover. Keep ownership until old character data is saved.
		if _, pending := s.kickAt[old]; !pending {
			if old.CharName != "" {
				handler.SendServerMessage(old, 357)
			}
			old.SetState(packet.StateDisconnecting)
			old.FlushOutput()
			s.kickAt[old] = s.now().Add(time.Second)
		}
		return handler.ErrLoginCollision
	}
	claimed, err := s.repo.TryMarkOnline(ctx, account.Name)
	if err != nil {
		// A timeout may arrive AFTER PostgreSQL committed the update. Reserve
		// cleanup even though this connection has not authenticated; otherwise
		// an ambiguous successful claim would strand the account until restart.
		if !errors.Is(err, persist.ErrAccountUnavailable) {
			s.owners[account.Name] = sess
			s.retryAt[account.Name] = s.now()
		}
		return fmt.Errorf("mark account online: %w", err)
	}
	if !claimed {
		return handler.ErrAccountOnline
	}
	// Register even if the socket closed during the database call, so the input
	// loop can release this claim. Never reassign an existing owner.
	sess.AccountName = account.Name
	s.owners[account.Name] = sess
	return nil
}

func (s *LoginSystem) onlineCount() int {
	count := 0
	for name, sess := range s.owners {
		if !sess.IsClosed() && sess.AccountName == name {
			count++
		}
	}
	return count
}

// Logout runs AFTER character/inventory cleanup. Only the current owner can
// clear the database flag; duplicate or delayed cleanup of old sessions is inert.
func (s *LoginSystem) Logout(sess *net.Session) {
	if sess.AccountName == "" || s.owners[sess.AccountName] != sess {
		return
	}
	delete(s.kickAt, sess)
	s.markOffline(sess.AccountName, sess)
}

func (s *LoginSystem) markOffline(name string, sess *net.Session) {
	if s.owners[name] != sess {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.repo.SetOnline(ctx, name, false); err != nil {
		s.log.Error("清除帳號在線狀態失敗，將重試", zap.String("account", name), zap.Error(err))
		s.retryAt[name] = s.now().Add(time.Second)
		return
	}
	delete(s.owners, name)
	delete(s.retryAt, name)
}

// Tick is called before dispatch/cleanup, including high-frequency input polls.
func (s *LoginSystem) Tick() {
	now := s.now()
	for sess, deadline := range s.kickAt {
		if sess.IsClosed() || !now.Before(deadline) {
			sess.Close()
			delete(s.kickAt, sess)
		}
	}
	for name, deadline := range s.retryAt {
		if !now.Before(deadline) {
			s.markOffline(name, s.owners[name])
			// Bound work per input poll when the database is unavailable so
			// retries cannot multiply one tick's timeout by the account count.
			break
		}
	}
}

// RetryOffline is a final bounded attempt during orderly shutdown.
func (s *LoginSystem) RetryOffline() error {
	for name := range s.retryAt {
		s.markOffline(name, s.owners[name])
	}
	if len(s.retryAt) != 0 {
		return fmt.Errorf("%d accounts still need offline cleanup", len(s.retryAt))
	}
	return nil
}
