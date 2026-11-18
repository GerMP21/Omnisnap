package cli

import (
	"context"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// env is the machine the commands run on; tests replace it.
type env struct {
	find   func(context.Context) ([]discovery.Browser, error)
	launch func(context.Context, discovery.Browser) (engine, error)
}

// engine is one running browser: one per engine for the whole job.
type engine interface {
	NewSession(ctx context.Context) (drivers.Session, error)
	Close(ctx context.Context) error
}

func defaultEnv() env {
	return env{
		find: func(ctx context.Context) ([]discovery.Browser, error) {
			return discovery.Find(ctx, discovery.Options{})
		},
		launch: launch,
	}
}

// canDrive says which families have a driver. Gecko (BiDi) and WebKit
// (Classic) arrive in Phase 3 (Tasks 16-23).
func canDrive(b discovery.Browser) bool { return b.Family == schema.FamilyBlink }

func launch(ctx context.Context, b discovery.Browser) (engine, error) {
	if !canDrive(b) {
		return nil, fmt.Errorf("%s: %s engines are not supported yet", b.ID, b.Family)
	}
	p, err := process.StartBlink(ctx, b.Executable, "")
	if err != nil {
		return nil, err
	}
	c, err := cdp.Dial(ctx, p.WSURL)
	if err != nil {
		_ = p.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return &blinkEngine{proc: p, cdp: c}, nil
}

type blinkEngine struct {
	proc *process.Blink
	cdp  *cdp.Browser
}

func (e *blinkEngine) NewSession(ctx context.Context) (drivers.Session, error) {
	return e.cdp.NewSession(ctx)
}

func (e *blinkEngine) Close(ctx context.Context) error {
	_ = e.cdp.Close()
	return e.proc.Close(ctx)
}
