package cli

import (
	"context"
	"sync"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
)

// restartable is one engine's browser for the job, which the crash policy can
// replace (capture.Engine.Relaunch): the dead browser is closed and a fresh
// one launched the way the first was.
type restartable struct {
	b      discovery.Browser
	launch func(context.Context, discovery.Browser) (engine, error)

	mu  sync.Mutex
	cur engine // nil after a relaunch that failed
}

func (r *restartable) NewSession(ctx context.Context) (drivers.Session, error) {
	r.mu.Lock()
	cur := r.cur
	r.mu.Unlock()
	if cur == nil {
		return nil, drivers.ErrCrashed
	}
	return cur.NewSession(ctx)
}

func (r *restartable) Relaunch(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur != nil {
		_ = r.cur.Close(context.WithoutCancel(ctx))
		r.cur = nil
	}
	eng, err := r.launch(ctx, r.b)
	if err != nil {
		return err
	}
	r.cur = eng
	return nil
}

func (r *restartable) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur == nil {
		return nil
	}
	err := r.cur.Close(ctx)
	r.cur = nil
	return err
}
