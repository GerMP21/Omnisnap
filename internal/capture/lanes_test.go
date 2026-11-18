package capture

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// order records "<engine> <run>" as captures happen.
type order struct {
	mu  sync.Mutex
	got []string
}

func (o *order) add(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.got = append(o.got, s)
}

func (o *order) list() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.got...)
}

// engineFor builds a capture.Engine whose sessions record their order and
// optionally meet at a barrier in SetViewport.
func engineFor(id string, family schema.Family, o *order, barrier *sync.WaitGroup, ready chan<- string) Engine {
	return Engine{
		ID: id, Family: family, Version: "test",
		NewSession: func(context.Context) (drivers.Session, error) {
			s := newFake()
			return &recordingSession{fakeSession: s, id: id, o: o, barrier: barrier, ready: ready}, nil
		},
	}
}

// recordingSession reports when a capture starts, and can hold every lane at a
// barrier until all have arrived (proving the lanes overlap).
type recordingSession struct {
	*fakeSession
	id      string
	o       *order
	barrier *sync.WaitGroup
	ready   chan<- string
	once    sync.Once
}

func (s *recordingSession) SetViewport(ctx context.Context, w, h int, dpr float64) (drivers.Viewport, error) {
	s.once.Do(func() {
		if s.ready != nil {
			s.ready <- s.id
		}
	})
	if s.barrier != nil {
		s.barrier.Done()
		done := make(chan struct{})
		go func() { s.barrier.Wait(); close(done) }()
		select {
		case <-done:
		case <-ctx.Done():
			return drivers.Viewport{}, ctx.Err()
		}
	}
	return s.fakeSession.SetViewport(ctx, w, h, dpr)
}

// TestRunPageOrder proves all run a before any run b within a lane.
func TestRunPageOrder(t *testing.T) {
	o := &order{}
	plan := Plan{Out: t.TempDir(), Viewport: schema.DefaultViewport, Runs: 2, Engines: []Engine{
		engineFor("one", schema.FamilyBlink, o, nil, nil),
		engineFor("two", schema.FamilyBlink, o, nil, nil),
	}}
	var mu sync.Mutex
	var got []string
	err := RunPage(context.Background(), plan, schema.PageRef{Slug: "p", URL: "https://example.com/"}, func(out Outcome) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, out.Engine+" "+out.Run)
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"one a", "two a", "one b", "two b"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("order %v; want %v (all run a before any run b)", got, want)
	}
}

// TestLanesOverlap proves the Classic lane and the CDP/BiDi lane capture at
// the same time: each waits at a barrier only both can release.
func TestLanesOverlap(t *testing.T) {
	var barrier sync.WaitGroup
	barrier.Add(2)
	ready := make(chan string, 2)
	plan := Plan{Out: t.TempDir(), Viewport: schema.DefaultViewport, Runs: 1, Engines: []Engine{
		engineFor("blink", schema.FamilyBlink, &order{}, &barrier, ready),
		engineFor("webkit", schema.FamilyWebKit, &order{}, &barrier, ready),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunPage(ctx, plan, schema.PageRef{Slug: "p", URL: "https://example.com/"}, func(Outcome) {})
	}()
	// Both lanes must arrive: without overlap the second never starts and the
	// barrier times out.
	for range 2 {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("lanes did not overlap: only one ever started a capture")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestRunPageReportsFailuresAndContinues: one engine failing does not stop the
// other captures (the failure policy, Task 26, decides what to do with it).
func TestRunPageReportsFailuresAndContinues(t *testing.T) {
	failing := Engine{ID: "bad", Family: schema.FamilyBlink, Version: "test",
		NewSession: func(context.Context) (drivers.Session, error) {
			return nil, errors.New("no browser")
		}}
	plan := Plan{Out: t.TempDir(), Viewport: schema.DefaultViewport, Runs: 1, Engines: []Engine{
		failing,
		engineFor("good", schema.FamilyBlink, &order{}, nil, nil),
	}}
	outcomes := map[string]error{}
	err := RunPage(context.Background(), plan, schema.PageRef{Slug: "p", URL: "https://example.com/"}, func(o Outcome) {
		outcomes[o.Engine] = o.Err
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcomes["bad"] == nil || outcomes["good"] != nil {
		t.Errorf("outcomes %v; want bad failed, good captured", outcomes)
	}
}
