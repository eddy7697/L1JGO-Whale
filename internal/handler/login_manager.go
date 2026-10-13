package handler

import (
	"context"
	"errors"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/persist"
)

var (
	ErrAccountOnline  = errors.New("account is already online")
	ErrServerFull     = errors.New("maximum online accounts reached")
	ErrLoginCollision = errors.New("account has another session")
)

// LoginManager admits an already authenticated, unbanned account on the game loop.
type LoginManager interface {
	Login(context.Context, *net.Session, *persist.AccountRow) error
}
