package net

import (
	stdnet "net"
	"sync"
	"testing"

	"github.com/l1jgo/server/internal/net/packet"
	"go.uber.org/zap"
)

func TestSessionCloseCannotBeUndoneByAuthentication(t *testing.T) {
	for i := 0; i < 100; i++ {
		server, client := stdnet.Pipe()
		s := NewSession(server, 1, 1, 1, 0, zap.NewNop())
		s.SetState(packet.StateVersionOK)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); s.Close() }()
		go func() { defer wg.Done(); s.SetState(packet.StateAuthenticated) }()
		wg.Wait()
		s.SetState(packet.StateInWorld) // simulate a late handler completing a DB call
		if !s.IsClosed() || s.State() != packet.StateDisconnecting {
			t.Fatalf("closed session returned to %v", s.State())
		}
		client.Close()
	}
}
