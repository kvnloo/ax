// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/ax/internal/store"
	"github.com/google/ax/pkg/apis/v1alpha1"
)

const (
	defaultWorkerGroup = "ax-controllers"
	// readRetryDelay is how long the worker waits after a transient error from the
	// event queue before trying again.
	readRetryDelay = time.Second
	// readinessRequeueDelay is the default delay before the worker republishes a
	// reconcile event for a task whose reconcile finished without reaching a
	// ready state (workspace still initializing, or actor resumed without a
	// worker assignment). Reconcile is purely event-driven: without this, a
	// workspace setup outlasting the in-reconcile poll wedges the task in
	// Initializing until an unrelated update arrives.
	readinessRequeueDelay = 10 * time.Second
	// requeueStoreTimeout bounds the re-read plus republish a delayed requeue
	// performs.
	requeueStoreTimeout = 10 * time.Second
)

// Worker consumes task events from the store's event queue and reconciles each
// task against Substrate. Run several with the same group name to share the load;
// every event is handled by exactly one of them.
type Worker struct {
	store      store.Store
	reconciler *TaskReconciler
	group      string
	consumer   string
	// requeueDelay overrides readinessRequeueDelay; tests set it small.
	requeueDelay time.Duration
}

// NewWorker creates a worker that joins group as consumer. An empty group uses the
// default controller group; an empty consumer derives a unique name from the host.
func NewWorker(s store.Store, reconciler *TaskReconciler, group, consumer string) *Worker {
	if group == "" {
		group = defaultWorkerGroup
	}
	if consumer == "" {
		hostname, _ := os.Hostname()
		consumer = fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano()%10000)
	}
	return &Worker{
		store:        s,
		reconciler:   reconciler,
		group:        group,
		consumer:     consumer,
		requeueDelay: readinessRequeueDelay,
	}
}

// Run subscribes to task events and processes them until ctx is done. It returns
// ctx.Err() on shutdown; every event is acknowledged after processing, even when
// reconciliation fails, so a bad task cannot wedge the queue.
func (w *Worker) Run(ctx context.Context) error {
	slog.Info("starting AX task worker", "group", w.group, "consumer", w.consumer)

	sub, err := w.store.Subscribe(ctx, w.group, w.consumer)
	if err != nil {
		return fmt.Errorf("subscribing to task events: %w", err)
	}
	defer sub.Close()

	for {
		ev, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("stopping AX task worker")
				return ctx.Err()
			}
			slog.Error("error reading task events", "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(readRetryDelay):
			}
			continue
		}

		if err := w.processEvent(ctx, ev); err != nil {
			slog.Error("error processing task event",
				"id", ev.ID,
				"atespace", ev.Atespace,
				"name", ev.Name,
				"action", ev.Action,
				"error", err,
			)
		}
		if err := sub.Ack(ctx, ev); err != nil {
			slog.Warn("failed to acknowledge task event", "id", ev.ID, "error", err)
		}
	}
}

func (w *Worker) processEvent(ctx context.Context, ev store.TaskEvent) error {
	if ev.Action == "delete" {
		return w.deleteTask(ctx, ev.Atespace, ev.Name)
	}

	task, err := w.store.GetTask(ctx, ev.Atespace, ev.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			slog.Info("task not found, skipping reconcile", "atespace", ev.Atespace, "name", ev.Name)
			return nil
		}
		return fmt.Errorf("fetching task %s/%s: %w", ev.Atespace, ev.Name, err)
	}

	if task.Status != nil && task.Status.Phase == v1alpha1.PhaseTerminating {
		// A Terminating task is past the point of no return: a "reconcile"
		// event can still arrive for it when an update races the delete (every
		// SaveTask publishes one). Running the normal path here would call
		// ResumeActor and resurrect an actor that is being torn down, so the
		// delete path wins regardless of the event action.
		slog.Info("task is terminating; completing deletion instead of reconciling",
			"atespace", ev.Atespace, "name", ev.Name)
		return w.deleteTask(ctx, ev.Atespace, ev.Name)
	}

	var gw *v1alpha1.Gateway
	if task.Spec.Gateway != nil && task.Spec.Gateway.Name != "" {
		g, err := w.store.GetGateway(ctx, task.Metadata.Atespace, task.Spec.Gateway.Name)
		if err == nil {
			gw = g
		} else if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("error fetching gateway", "name", task.Spec.Gateway.Name, "error", err)
		}
	}

	// Resolve every bound workspace. A missing one is skipped so the task still
	// runs; the runner creates an empty directory at its path.
	var workspaces []*v1alpha1.Workspace
	for _, ref := range task.Spec.WorkspaceRefs() {
		if ref.Name == "" {
			continue
		}
		wsp, err := w.store.GetWorkspace(ctx, task.Metadata.Atespace, ref.Name)
		if err == nil {
			workspaces = append(workspaces, wsp)
		} else if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("error fetching workspace", "name", ref.Name, "error", err)
		}
	}

	reconciled, err := w.reconciler.Reconcile(ctx, task, gw, workspaces...)
	if err != nil {
		task.Status.Phase = "Failed"
		_ = w.store.UpdateTaskStatus(ctx, task.Metadata.Atespace, task.Metadata.Name, task.Status)
		return fmt.Errorf("reconciling task %s/%s: %w", task.Metadata.Atespace, task.Metadata.Name, err)
	}

	// Snapshot the requeue decision BEFORE UpdateTaskStatus: the store adopts
	// the status pointer, so reading it afterwards would race a concurrent
	// MarkTaskDeleting mutating the same struct.
	needRequeue := w.readinessRequeueNeeded(reconciled)

	if err := w.store.UpdateTaskStatus(ctx, task.Metadata.Atespace, task.Metadata.Name, reconciled.Status); err != nil {
		return fmt.Errorf("updating task status %s/%s: %w", task.Metadata.Atespace, task.Metadata.Name, err)
	}

	// A reconcile that leaves the task still initializing (or still waiting
	// for a worker assignment) gets no further events on its own: republish a
	// reconcile event after a delay so the readiness poll runs again.
	if needRequeue {
		w.scheduleRequeue(ctx, ev.Atespace, ev.Name)
	}

	return nil
}

// readinessRequeueNeeded reports whether a reconciled task still needs its
// readiness polled: running with workspace setup unfinished, or resumed but
// not yet assigned to a worker. Anything else (suspended, failed, complete,
// terminating) must not be requeued.
func (w *Worker) readinessRequeueNeeded(task *v1alpha1.Task) bool {
	if task == nil || task.Status == nil {
		return false
	}
	switch task.Status.Phase {
	case "Running":
		return !w.reconciler.conditionTrue(task, condWorkspaceReady)
	case "Pending":
		// The empty-workerIP path: the actor is resumed but Substrate has not
		// assigned it to a worker yet. Nothing else triggers a reconcile when
		// the assignment lands, so poll again.
		return task.Status.WorkerIp == ""
	default:
		return false
	}
}

// scheduleRequeue republishes a reconcile event after the worker's requeue
// delay so a still-initializing task is polled again. The task is re-read
// first: a task that became ready, was suspended, failed, or was deleted in
// the meantime is not requeued, and a deleted task is never resurrected by
// the republish.
func (w *Worker) scheduleRequeue(ctx context.Context, atespace, name string) {
	delay := w.requeueDelay
	if delay <= 0 {
		delay = readinessRequeueDelay
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		rctx, cancel := context.WithTimeout(context.Background(), requeueStoreTimeout)
		defer cancel()
		task, err := w.store.GetTask(rctx, atespace, name)
		if err != nil {
			return
		}
		if !w.readinessRequeueNeeded(task) {
			return
		}
		// SaveTask republishes the reconcile event; the status write itself is
		// unchanged content.
		if err := w.store.SaveTask(rctx, task); err != nil {
			slog.Warn("failed to republish reconcile event for initializing task",
				"atespace", atespace, "name", name, "error", err)
		}
	}()
}

// deleteTask tears down the Substrate actor and removes the task record. The
// record is left in Terminating when cleanup fails so the failure stays
// visible; re-running `ax delete` republishes the event and retries.
func (w *Worker) deleteTask(ctx context.Context, atespace, name string) error {
	slog.Info("handling task deletion event", "atespace", atespace, "name", name)
	if err := w.reconciler.ReconcileDelete(ctx, atespace, name); err != nil {
		return fmt.Errorf("cleaning up task %s/%s: %w", atespace, name, err)
	}
	if err := w.store.DeleteTask(ctx, atespace, name); err != nil {
		return fmt.Errorf("removing task record %s/%s: %w", atespace, name, err)
	}
	return nil
}
