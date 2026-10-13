package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type leasePingFunc func(context.Context) error

func (fn leasePingFunc) Ping(ctx context.Context) error { return fn(ctx) }

func TestOnlineLeaseMonitorReportsFailure(t *testing.T) {
	want := errors.New("synthetic lease loss")
	failures, stop := watchOnlineLease(leasePingFunc(func(context.Context) error { return want }), time.Millisecond, time.Second)
	defer stop()
	select {
	case err := <-failures:
		if err != want {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("lease failure not reported")
	}
}

func TestOnlineLeaseMonitorStopWaitsForInflightPing(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	failures, stop := watchOnlineLease(leasePingFunc(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	}), time.Millisecond, time.Second)
	defer stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("ping did not start")
	}
	stop()
	select {
	case <-finished:
	default:
		t.Fatal("stop returned before ping exited")
	}
	select {
	case err := <-failures:
		t.Fatalf("normal shutdown reported as lease loss: %v", err)
	default:
	}
}
