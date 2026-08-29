package hooks

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestHermesSessionClaimIsAtomicAndBounded(t *testing.T) {
	root := t.TempDir()
	var claimed atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			first, err := claimHermesSession(root, "shared-session")
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if first {
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("shared session claimed %d times", claimed.Load())
	}
	for i := 0; i < hermesMarkerMax+40; i++ {
		if _, err := claimHermesSession(root, string(rune(i))+"-unique"); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(hermesMarkerDir)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > hermesMarkerMax {
		t.Fatalf("Hermes marker count = %d, max %d", len(entries), hermesMarkerMax)
	}
}

func TestHermesSessionStateCannotEscapeKnowledgeBase(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".state", "hooks")); err != nil {
		t.Fatal(err)
	}
	if claimed, err := claimHermesSession(root, "escape"); err == nil || claimed {
		t.Fatalf("escaping state claim = %v, err=%v", claimed, err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside state changed: entries=%v err=%v", entries, err)
	}
}
