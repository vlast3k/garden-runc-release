package peas_test

import (
	"errors"
	"testing"
	"time"

	"code.cloudfoundry.org/guardian/rundmc/peas"
	"code.cloudfoundry.org/guardian/rundmc/peas/peasfakes"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
)

type retainedPeaRuntime struct {
	peasfakes.FakeRuntime
	handles []string
	err     error
	queries int
}

func (r *retainedPeaRuntime) PeaHandles() ([]string, error) {
	r.queries++
	return r.handles, r.err
}

func TestPeaCleanerDiscoversChildrenWithoutParentMetadata(t *testing.T) {
	runtime := &retainedPeaRuntime{handles: []string{"retained-child", "live-child"}}
	// No parent metadata exists. The historical parent-first scan finds nothing.
	done := make(chan string, 2)
	cleaner := &peas.PeaCleaner{Runtime: runtime, TaskWaitAndCleaner: func(_ lager.Logger, id string) error {
		done <- id
		return nil
	}}
	if err := cleaner.CleanAll(lagertest.NewTestLogger("retained-peas")); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range runtime.handles {
		select {
		case id := <-done:
			if seen[id] {
				t.Fatalf("duplicate cleanup %q", id)
			}
			seen[id] = true
		case <-time.After(100 * time.Millisecond):
			t.Fatal("parentless pea was not passed to shared task cleanup")
		}
	}
	if !seen["retained-child"] || !seen["live-child"] || runtime.queries != 1 || runtime.ContainerHandlesCallCount() != 0 || runtime.ContainerPeaHandlesCallCount() != 0 {
		t.Fatalf("wrong discovery: seen=%v queries=%d parent=%d child=%d", seen, runtime.queries, runtime.ContainerHandlesCallCount(), runtime.ContainerPeaHandlesCallCount())
	}
}

func TestPeaCleanerPropagatesDirectDiscoveryFailure(t *testing.T) {
	want := errors.New("pea metadata unavailable")
	runtime := &retainedPeaRuntime{handles: []string{"unsafe-partial-result"}, err: want}
	done := make(chan string, 1)
	cleaner := &peas.PeaCleaner{Runtime: runtime, TaskWaitAndCleaner: func(_ lager.Logger, id string) error { done <- id; return nil }}
	if err := cleaner.CleanAll(lagertest.NewTestLogger("retained-peas")); !errors.Is(err, want) {
		t.Fatalf("lost discovery error: %v", err)
	}
	select {
	case id := <-done:
		t.Fatalf("cleaned partial discovery %s", id)
	default:
	}
}

func TestPeaCleanerRequiresDirectDiscoveryForTaskCleanup(t *testing.T) {
	cleaner := &peas.PeaCleaner{Runtime: new(peasfakes.FakeRuntime), TaskWaitAndCleaner: func(lager.Logger, string) error { return nil }}
	if err := cleaner.CleanAll(lagertest.NewTestLogger("retained-peas")); err == nil {
		t.Fatal("silently fell back to incomplete parent discovery")
	}
}
