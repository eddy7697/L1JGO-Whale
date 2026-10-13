package handler

import (
	"encoding/binary"
	stdnet "net"
	"strings"
	"testing"

	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"go.uber.org/zap"
)

func TestLoginMalformedCredentialsNeverReachDatabase(t *testing.T) {
	for _, payload := range []string{
		"", "\x00password\x00", "account\x00", "account\x00unterminated",
		"bad space\x00password\x00", strings.Repeat("a", 33) + "\x00password\x00",
		"account\x00" + strings.Repeat("x", 33) + "\x00",
	} {
		t.Run(fmtAuthTestName(payload), func(t *testing.T) {
			server, client := stdnet.Pipe()
			defer client.Close()
			s := l1net.NewSession(server, 1, 16, 16, 0, zap.NewNop())
			defer s.Close()
			s.SetState(packet.StateVersionOK)
			// nil dependencies prove rejection happens before account creation/load.
			HandleAuthDirect(s, packet.NewReader(append([]byte{packet.C_OPCODE_LOGIN}, []byte(payload)...)), nil)
			s.FlushOutput()
			select {
			case p := <-s.OutQueue:
				if p[0] != packet.S_OPCODE_LOGIN_CHECK || binary.LittleEndian.Uint16(p[1:3]) != 8 {
					t.Fatalf("invalid login response %v", p)
				}
			default:
				t.Fatal("missing invalid credential response")
			}
			if s.AccountName != "" || s.State() != packet.StateVersionOK {
				t.Fatal("malformed login changed account state")
			}
		})
	}
}

func fmtAuthTestName(s string) string { return strings.ReplaceAll(s, "\x00", "|") }

func TestLoginAuthenticatedSessionCannotSwitchAccount(t *testing.T) {
	for _, st := range []packet.SessionState{packet.StateAuthenticated, packet.StateInWorld, packet.StateReturningToSelect, packet.StateDisconnecting} {
		t.Run(st.String(), func(t *testing.T) {
			server, client := stdnet.Pipe()
			defer client.Close()
			s := l1net.NewSession(server, 1, 16, 16, 0, zap.NewNop())
			defer s.Close()
			s.AccountName = "owner"
			s.SetState(st)
			w := packet.NewWriterWithOpcode(packet.C_OPCODE_SHIFT_SERVER)
			w.WriteC(6)
			w.WriteS("other")
			w.WriteS("synthetic")
			HandleAuthBeanFun(s, packet.NewReader(w.Bytes()), nil)
			if s.AccountName != "owner" || s.State() != st {
				t.Fatal("session switched account")
			}
		})
	}
}
