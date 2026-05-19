package lock

import (
	"os"
	"testing"
	"time"

	"github.com/danew/devsync/internal/apperrors"
)

func TestAcquireRejectsHeldLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	first, err := Acquire("steel-api", "sync")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	second, err := Acquire("steel-api", "sync")
	if second != nil {
		defer second.Release()
	}
	if !apperrors.Is(err, apperrors.ErrWorkspaceLockHeld) {
		t.Fatalf("expected ErrWorkspaceLockHeld, got %v", err)
	}
}

func TestAcquireRecoversStaleLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := Path("steel-api")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path[:len(path)-len("steel-api.lock")], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	lock, err := Acquire("steel-api", "sync")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
}
