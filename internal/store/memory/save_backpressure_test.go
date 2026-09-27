package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/ax/internal/store/memory"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

func saveTask(t *testing.T, st *memory.MemoryStore, ctx context.Context) {
	t.Helper()
	task := &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"}}
	if err := st.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
}

// A full event buffer with a canceled caller context must surface the
// cancellation instead of silently dropping the reconcile event.
func TestSaveTaskFullBufferCanceledCtx(t *testing.T) {
	st := memory.NewStore()
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		saveTask(t, st, ctx)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	task := &v1alpha1.Task{Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"}}
	if err := st.SaveTask(canceled, task); !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveTask on full buffer with canceled ctx = %v, want context.Canceled (event silently dropped)", err)
	}
}

// With room in the buffer the reconcile event is still published and
// consumable through a subscription.
func TestSaveTaskPublishesEvent(t *testing.T) {
	st := memory.NewStore()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	saveTask(t, st, ctx)

	sub, err := st.Subscribe(ctx, "g", "c")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ev, err := sub.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if ev.Action != "reconcile" || ev.Name != "t" {
		t.Fatalf("event = %+v, want reconcile for t", ev)
	}
}
