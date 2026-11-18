package cli

import (
	"context"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/branded"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/install"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"path/filepath"
	"time"
)

// env is the machine the commands run on; tests replace it.
type env struct {
	find   func(context.Context) ([]discovery.Browser, error)
	launch func(context.Context, discovery.Browser) (engine, error)
	cache  func() (string, error) // omnisnap's browser cache (install/uninstall)
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
		cache:  func() (string, error) { return install.CacheDir(nil) },
	}
}

// canDrive says which families have a driver: one protocol per family, all
// three since Phase 3 (SPEC-drivers.md).
func canDrive(b discovery.Browser) bool {
	switch b.Family {
	case schema.FamilyBlink, schema.FamilyGecko, schema.FamilyWebKit:
		return true
	}
	return false
}

func launch(ctx context.Context, b discovery.Browser) (engine, error) {
	if !canDrive(b) {
		return nil, fmt.Errorf("%s: %s engines are not supported", b.ID, b.Family)
	}
	switch b.Family {
	case schema.FamilyGecko:
		return launchGecko(ctx, b)
	case schema.FamilyWebKit:
		return launchWebKit(ctx, b)
	default:
		return launchBlink(ctx, b)
	}
}

func launchBlink(ctx context.Context, b discovery.Browser) (engine, error) {
	// Branded browsers run from a copy of their warmed template when one is
	// fresh (SPEC-branded.md): a fresh profile has not downloaded its
	// blocking lists and would behave like Chrome.
	p, err := startBlink(ctx, b)
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

// startBlink uses the warmed template for branded browsers, else a fresh
// profile.
func startBlink(ctx context.Context, b discovery.Browser) (*process.Blink, error) {
	cache, err := install.CacheDir(nil)
	if err == nil {
		if t, ferr := branded.Find(filepath.Dir(cache), b.ID, b.Version, time.Now()); ferr == nil && t != nil {
			return process.StartBlinkTemplate(ctx, b.Executable, "", t.Dir)
		}
	}
	return process.StartBlink(ctx, b.Executable, "")
}

func launchGecko(ctx context.Context, b discovery.Browser) (engine, error) {
	p, err := process.StartGecko(ctx, b.Executable, "")
	if err != nil {
		return nil, err
	}
	c, err := bidi.Dial(ctx, p.WSURL)
	if err != nil {
		_ = p.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return &geckoEngine{proc: p, bidi: c}, nil
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

type geckoEngine struct {
	proc *process.Gecko
	bidi *bidi.Browser
}

func (e *geckoEngine) NewSession(ctx context.Context) (drivers.Session, error) {
	return e.bidi.NewSession(ctx)
}

func (e *geckoEngine) Close(ctx context.Context) error {
	_ = e.bidi.Close(ctx)
	return e.proc.Close(ctx)
}

// webkitEngine drives WebKit over Classic; stop closes the driver (and, on
// Linux, its private display).
type webkitEngine struct {
	classic *classic.Client
	stop    func(context.Context) error
}

func (e *webkitEngine) NewSession(ctx context.Context) (drivers.Session, error) {
	return e.classic.NewSession(ctx)
}

func (e *webkitEngine) Close(ctx context.Context) error { return e.stop(ctx) }
