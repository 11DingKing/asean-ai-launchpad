package health

import (
	"context"
	"sync/atomic"
)

type Pinger interface {
	Ping(context.Context) error
}

type Checker struct {
	Store Pinger
	live  atomic.Bool
}

func New(store Pinger) *Checker {
	checker := &Checker{Store: store}
	checker.live.Store(true)
	return checker
}

func (c *Checker) Live() bool { return c.live.Load() }

func (c *Checker) Stop() { c.live.Store(false) }

func (c *Checker) Ready(ctx context.Context) error {
	if !c.Live() {
		return context.Canceled
	}
	return c.Store.Ping(ctx)
}
