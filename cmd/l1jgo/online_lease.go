package main

import (
	"context"
	"time"
)

type onlineLeasePinger interface{ Ping(context.Context) error }

// The dedicated lease connection has exactly one user while monitoring. stop
// waits for that user to exit before its caller closes the connection.
func watchOnlineLease(lease onlineLeasePinger, interval, timeout time.Duration) (<-chan error, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	failures := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, stop := context.WithTimeout(ctx, timeout)
				err := lease.Ping(pingCtx)
				stop()
				if err != nil {
					if ctx.Err() == nil {
						failures <- err
					}
					return
				}
			}
		}
	}()
	return failures, func() { cancel(); <-done }
}
