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

package redis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/ax/internal/store"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	defaultStreamName    = "ax:stream:tasks"
	defaultReadBatchSize = 10
	defaultReadBlock     = 2 * time.Second
	// defaultClaimMinIdle bounds how long a stream entry may sit
	// unacknowledged in the consumer group's pending list before an idle
	// subscriber reclaims it. It must exceed worst-case event processing
	// (a ~15s workspace-ready poll plus Substrate round trips); a slow but
	// live consumer is never stolen from below this idle age.
	defaultClaimMinIdle = time.Minute
)

var (
	jsonMarshalOpts   = protojson.MarshalOptions{UseProtoNames: false, EmitUnpopulated: false}
	jsonUnmarshalOpts = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// Options contains configuration for the Redis store.
type Options struct {
	StreamName string
	KeyPrefix  string
	TTL        time.Duration // Optional TTL for task records

	// ReadBatchSize is how many events one XREADGROUP call may return.
	ReadBatchSize int64
	// ReadBlock is how long one XREADGROUP call waits for events before returning
	// empty. Shorter values make shutdown more responsive at the cost of more calls.
	ReadBlock time.Duration
	// ClaimMinIdle is how long a stream entry may sit unacknowledged in the
	// consumer group's pending list before an idle subscriber reclaims it
	// via XAUTOCLAIM. It must exceed the worst-case event processing time;
	// below it, a slow (not dead) consumer's in-flight event would be
	// processed twice. Delivery is at-least-once either way.
	ClaimMinIdle time.Duration
}

// Store is a Redis-backed implementation of store.Store.
type Store struct {
	client *redis.Client
	opts   Options
}

// NewStore creates a new Redis store.
func NewStore(client *redis.Client, opts Options) *Store {
	if opts.StreamName == "" {
		opts.StreamName = defaultStreamName
	}
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "ax"
	}
	if opts.ReadBatchSize <= 0 {
		opts.ReadBatchSize = defaultReadBatchSize
	}
	if opts.ReadBlock <= 0 {
		opts.ReadBlock = defaultReadBlock
	}
	if opts.ClaimMinIdle <= 0 {
		opts.ClaimMinIdle = defaultClaimMinIdle
	}
	return &Store{
		client: client,
		opts:   opts,
	}
}

func (s *Store) taskKey(atespace, name string) string {
	return fmt.Sprintf("%s:task:%s:%s", s.opts.KeyPrefix, atespace, name)
}

func (s *Store) gwKey(atespace, name string) string {
	return fmt.Sprintf("%s:gw:%s:%s", s.opts.KeyPrefix, atespace, name)
}

func (s *Store) gwIndexKey() string {
	return fmt.Sprintf("%s:gateways:index", s.opts.KeyPrefix)
}

func (s *Store) gwAtespaceIndexKey(atespace string) string {
	return fmt.Sprintf("%s:gateways:atespace:%s", s.opts.KeyPrefix, atespace)
}

func (s *Store) modelKey(atespace, name string) string {
	return fmt.Sprintf("%s:model:%s:%s", s.opts.KeyPrefix, atespace, name)
}

func (s *Store) modelIndexKey() string {
	return fmt.Sprintf("%s:models:index", s.opts.KeyPrefix)
}

func (s *Store) modelAtespaceIndexKey(atespace string) string {
	return fmt.Sprintf("%s:models:atespace:%s", s.opts.KeyPrefix, atespace)
}

func (s *Store) wsKey(atespace, name string) string {
	return fmt.Sprintf("%s:workspace:%s:%s", s.opts.KeyPrefix, atespace, name)
}

func (s *Store) wsIndexKey() string {
	return fmt.Sprintf("%s:workspaces:index", s.opts.KeyPrefix)
}

func (s *Store) wsAtespaceIndexKey(atespace string) string {
	return fmt.Sprintf("%s:workspaces:atespace:%s", s.opts.KeyPrefix, atespace)
}

func (s *Store) taskIndexKey() string {
	return fmt.Sprintf("%s:tasks:index", s.opts.KeyPrefix)
}

func (s *Store) taskAtespaceIndexKey(atespace string) string {
	return fmt.Sprintf("%s:tasks:atespace:%s", s.opts.KeyPrefix, atespace)
}

func (s *Store) taskPubSubChannel(atespace, name string) string {
	return fmt.Sprintf("%s:pubsub:task:%s:%s", s.opts.KeyPrefix, atespace, name)
}

// SaveTask stores or updates a task and publishes a reconcile event to the stream.
func (s *Store) SaveTask(ctx context.Context, task *v1alpha1.Task) error {
	if task.Metadata == nil {
		task.Metadata = &v1alpha1.ObjectMeta{}
	}
	if task.Metadata.Name == "" {
		return errors.New("task name is required")
	}
	if task.Metadata.Atespace == "" {
		task.Metadata.Atespace = "default"
	}
	if task.ApiVersion == "" {
		task.ApiVersion = v1alpha1.APIVersion
	}
	if task.Kind == "" {
		task.Kind = v1alpha1.KindTask
	}
	if task.Status == nil {
		task.Status = &v1alpha1.TaskStatus{}
	}
	if task.Status.Phase == "" {
		task.Status.Phase = "Pending"
	}

	data, err := protojson.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshaling task: %w", err)
	}

	atespace := task.Metadata.Atespace
	name := task.Metadata.Name
	score := float64(time.Now().UnixNano())
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.taskKey(atespace, name), data, s.opts.TTL)
	pipe.ZAdd(ctx, s.taskIndexKey(), redis.Z{Score: score, Member: member})
	pipe.ZAdd(ctx, s.taskAtespaceIndexKey(atespace), redis.Z{Score: score, Member: name})
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: s.opts.StreamName,
		Values: map[string]interface{}{
			"action":   "reconcile",
			"atespace": atespace,
			"name":     name,
		},
	})
	pipe.Publish(ctx, s.taskPubSubChannel(atespace, name), data)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("saving task to redis: %w", err)
	}
	return nil
}

// GetTask retrieves a task by atespace and name.
func (s *Store) GetTask(ctx context.Context, atespace, name string) (*v1alpha1.Task, error) {
	if atespace == "" {
		atespace = "default"
	}
	val, err := s.client.Get(ctx, s.taskKey(atespace, name)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("getting task from redis: %w", err)
	}

	var task v1alpha1.Task
	if err := jsonUnmarshalOpts.Unmarshal([]byte(val), &task); err != nil {
		return nil, fmt.Errorf("unmarshaling task: %w", err)
	}
	return &task, nil
}

// ListTasks lists tasks ordered by newest first.
func (s *Store) ListTasks(ctx context.Context, atespace string, limit, offset int64) ([]*v1alpha1.Task, error) {
	if limit <= 0 {
		limit = 50
	}
	start := offset
	stop := offset + limit - 1

	var members []string
	var err error

	if atespace == "" || atespace == "*" {
		members, err = s.client.ZRevRange(ctx, s.taskIndexKey(), start, stop).Result()
	} else {
		names, nErr := s.client.ZRevRange(ctx, s.taskAtespaceIndexKey(atespace), start, stop).Result()
		if nErr == nil {
			for _, n := range names {
				members = append(members, fmt.Sprintf("%s:%s", atespace, n))
			}
		}
		err = nErr
	}

	if err != nil {
		return nil, fmt.Errorf("listing task index: %w", err)
	}
	if len(members) == 0 {
		return []*v1alpha1.Task{}, nil
	}

	keys := make([]string, len(members))
	for i, m := range members {
		parts := strings.SplitN(m, ":", 2)
		if len(parts) == 2 {
			keys[i] = s.taskKey(parts[0], parts[1])
		} else {
			keys[i] = s.taskKey("default", m)
		}
	}

	vals, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("batch fetching tasks: %w", err)
	}

	tasks := make([]*v1alpha1.Task, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		var t v1alpha1.Task
		if err := jsonUnmarshalOpts.Unmarshal([]byte(str), &t); err == nil {
			tasks = append(tasks, &t)
		}
	}
	return tasks, nil
}

// UpdateTaskStatus updates only the status portion of a task.
func (s *Store) UpdateTaskStatus(ctx context.Context, atespace, name string, status *v1alpha1.TaskStatus) error {
	task, err := s.GetTask(ctx, atespace, name)
	if err != nil {
		return err
	}

	task.Status = status
	data, err := jsonMarshalOpts.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshaling task status: %w", err)
	}

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.taskKey(atespace, name), data, s.opts.TTL)
	pipe.Publish(ctx, s.taskPubSubChannel(atespace, name), data)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("updating task status in redis: %w", err)
	}
	return nil
}

// MarkTaskDeleting flips the task to the Terminating phase, notifies watchers, and
// publishes a delete event for the controller. The record stays until DeleteTask.
func (s *Store) MarkTaskDeleting(ctx context.Context, atespace, name string) error {
	if atespace == "" {
		atespace = "default"
	}
	task, err := s.GetTask(ctx, atespace, name)
	if err != nil {
		return err
	}
	if task.Status == nil {
		task.Status = &v1alpha1.TaskStatus{}
	}
	task.Status.Phase = v1alpha1.PhaseTerminating
	data, err := protojson.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshaling task: %w", err)
	}

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.taskKey(atespace, name), data, s.opts.TTL)
	pipe.Publish(ctx, s.taskPubSubChannel(atespace, name), data)
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: s.opts.StreamName,
		Values: map[string]interface{}{
			"action":   "delete",
			"atespace": atespace,
			"name":     name,
		},
	})
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("marking task deleting in redis: %w", err)
	}
	return nil
}

// DeleteTask removes the task record and its index entries. No event is published.
func (s *Store) DeleteTask(ctx context.Context, atespace, name string) error {
	if atespace == "" {
		atespace = "default"
	}
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.taskKey(atespace, name))
	pipe.ZRem(ctx, s.taskIndexKey(), member)
	pipe.ZRem(ctx, s.taskAtespaceIndexKey(atespace), name)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("deleting task from redis: %w", err)
	}
	return nil
}

// SaveGateway stores a gateway.
func (s *Store) SaveGateway(ctx context.Context, gw *v1alpha1.Gateway) error {
	if gw.Metadata == nil {
		gw.Metadata = &v1alpha1.ObjectMeta{}
	}
	if gw.Metadata.Name == "" {
		return errors.New("gateway name is required")
	}
	if gw.Metadata.Atespace == "" {
		gw.Metadata.Atespace = "default"
	}
	if gw.ApiVersion == "" {
		gw.ApiVersion = v1alpha1.APIVersion
	}
	if gw.Kind == "" {
		gw.Kind = v1alpha1.KindGateway
	}

	data, err := protojson.Marshal(gw)
	if err != nil {
		return fmt.Errorf("marshaling gateway: %w", err)
	}

	atespace := gw.Metadata.Atespace
	name := gw.Metadata.Name
	score := float64(time.Now().UnixNano())
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.gwKey(atespace, name), data, 0)
	pipe.ZAdd(ctx, s.gwIndexKey(), redis.Z{Score: score, Member: member})
	pipe.ZAdd(ctx, s.gwAtespaceIndexKey(atespace), redis.Z{Score: score, Member: name})
	_, err = pipe.Exec(ctx)
	return err
}

// DeleteGateway removes a gateway.
func (s *Store) DeleteGateway(ctx context.Context, atespace, name string) error {
	if atespace == "" {
		atespace = "default"
	}
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.gwKey(atespace, name))
	pipe.ZRem(ctx, s.gwIndexKey(), member)
	pipe.ZRem(ctx, s.gwAtespaceIndexKey(atespace), name)
	_, err := pipe.Exec(ctx)
	return err
}

// ListGateways lists gateways for an atespace or across all atespaces.
func (s *Store) ListGateways(ctx context.Context, atespace string) ([]*v1alpha1.Gateway, error) {
	var members []string
	var err error

	if atespace == "" || atespace == "*" {
		members, err = s.client.ZRevRange(ctx, s.gwIndexKey(), 0, -1).Result()
	} else {
		names, nErr := s.client.ZRevRange(ctx, s.gwAtespaceIndexKey(atespace), 0, -1).Result()
		if nErr == nil {
			for _, n := range names {
				members = append(members, fmt.Sprintf("%s:%s", atespace, n))
			}
		}
		err = nErr
	}

	if err != nil {
		return nil, fmt.Errorf("listing gateway index: %w", err)
	}
	if len(members) == 0 {
		return []*v1alpha1.Gateway{}, nil
	}

	keys := make([]string, len(members))
	for i, m := range members {
		parts := strings.SplitN(m, ":", 2)
		if len(parts) == 2 {
			keys[i] = s.gwKey(parts[0], parts[1])
		} else {
			keys[i] = s.gwKey("default", m)
		}
	}

	vals, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("batch fetching gateways: %w", err)
	}

	gateways := make([]*v1alpha1.Gateway, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		var gw v1alpha1.Gateway
		if err := jsonUnmarshalOpts.Unmarshal([]byte(str), &gw); err == nil {
			gateways = append(gateways, &gw)
		}
	}
	return gateways, nil
}

// GetGateway retrieves a gateway by atespace and name.
func (s *Store) GetGateway(ctx context.Context, atespace, name string) (*v1alpha1.Gateway, error) {
	if atespace == "" {
		atespace = "default"
	}
	val, err := s.client.Get(ctx, s.gwKey(atespace, name)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("getting gateway from redis: %w", err)
	}

	var gw v1alpha1.Gateway
	if err := jsonUnmarshalOpts.Unmarshal([]byte(val), &gw); err != nil {
		return nil, fmt.Errorf("unmarshaling gateway: %w", err)
	}
	return &gw, nil
}

// SaveModel stores a model.
func (s *Store) SaveModel(ctx context.Context, model *v1alpha1.Model) error {
	if model.Metadata == nil {
		model.Metadata = &v1alpha1.ObjectMeta{}
	}
	if model.Metadata.Name == "" {
		return errors.New("model name is required")
	}
	if model.Metadata.Atespace == "" {
		model.Metadata.Atespace = "default"
	}
	if model.ApiVersion == "" {
		model.ApiVersion = v1alpha1.APIVersion
	}
	if model.Kind == "" {
		model.Kind = v1alpha1.KindModel
	}

	data, err := protojson.Marshal(model)
	if err != nil {
		return fmt.Errorf("marshaling model: %w", err)
	}

	atespace := model.Metadata.Atespace
	name := model.Metadata.Name
	score := float64(time.Now().UnixNano())
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.modelKey(atespace, name), data, 0)
	pipe.ZAdd(ctx, s.modelIndexKey(), redis.Z{Score: score, Member: member})
	pipe.ZAdd(ctx, s.modelAtespaceIndexKey(atespace), redis.Z{Score: score, Member: name})
	_, err = pipe.Exec(ctx)
	return err
}

// DeleteModel removes a model.
func (s *Store) DeleteModel(ctx context.Context, atespace, name string) error {
	if atespace == "" {
		atespace = "default"
	}
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.modelKey(atespace, name))
	pipe.ZRem(ctx, s.modelIndexKey(), member)
	pipe.ZRem(ctx, s.modelAtespaceIndexKey(atespace), name)
	_, err := pipe.Exec(ctx)
	return err
}

// ListModels lists models for an atespace or across all atespaces.
func (s *Store) ListModels(ctx context.Context, atespace string) ([]*v1alpha1.Model, error) {
	var members []string
	var err error

	if atespace == "" || atespace == "*" {
		members, err = s.client.ZRevRange(ctx, s.modelIndexKey(), 0, -1).Result()
	} else {
		names, nErr := s.client.ZRevRange(ctx, s.modelAtespaceIndexKey(atespace), 0, -1).Result()
		if nErr == nil {
			for _, n := range names {
				members = append(members, fmt.Sprintf("%s:%s", atespace, n))
			}
		}
		err = nErr
	}

	if err != nil {
		return nil, fmt.Errorf("listing model index: %w", err)
	}
	if len(members) == 0 {
		return []*v1alpha1.Model{}, nil
	}

	keys := make([]string, len(members))
	for i, m := range members {
		parts := strings.SplitN(m, ":", 2)
		if len(parts) == 2 {
			keys[i] = s.modelKey(parts[0], parts[1])
		} else {
			keys[i] = s.modelKey("default", m)
		}
	}

	vals, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("batch fetching models: %w", err)
	}

	models := make([]*v1alpha1.Model, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		var m v1alpha1.Model
		if err := jsonUnmarshalOpts.Unmarshal([]byte(str), &m); err == nil {
			models = append(models, &m)
		}
	}
	return models, nil
}

// GetModel retrieves a model by atespace and name.
func (s *Store) GetModel(ctx context.Context, atespace, name string) (*v1alpha1.Model, error) {
	if atespace == "" {
		atespace = "default"
	}
	val, err := s.client.Get(ctx, s.modelKey(atespace, name)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("getting model from redis: %w", err)
	}

	var m v1alpha1.Model
	if err := jsonUnmarshalOpts.Unmarshal([]byte(val), &m); err != nil {
		return nil, fmt.Errorf("unmarshaling model: %w", err)
	}
	return &m, nil
}

// SaveWorkspace stores a workspace.
func (s *Store) SaveWorkspace(ctx context.Context, ws *v1alpha1.Workspace) error {
	if ws.Metadata == nil {
		ws.Metadata = &v1alpha1.ObjectMeta{}
	}
	if ws.Metadata.Name == "" {
		return errors.New("workspace name is required")
	}
	if ws.Metadata.Atespace == "" {
		ws.Metadata.Atespace = "default"
	}
	if ws.ApiVersion == "" {
		ws.ApiVersion = v1alpha1.APIVersion
	}
	if ws.Kind == "" {
		ws.Kind = v1alpha1.KindWorkspace
	}

	data, err := protojson.Marshal(ws)
	if err != nil {
		return fmt.Errorf("marshaling workspace: %w", err)
	}

	atespace := ws.Metadata.Atespace
	name := ws.Metadata.Name
	score := float64(time.Now().UnixNano())
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.wsKey(atespace, name), data, 0)
	pipe.ZAdd(ctx, s.wsIndexKey(), redis.Z{Score: score, Member: member})
	pipe.ZAdd(ctx, s.wsAtespaceIndexKey(atespace), redis.Z{Score: score, Member: name})
	_, err = pipe.Exec(ctx)
	return err
}

// DeleteWorkspace removes a workspace.
func (s *Store) DeleteWorkspace(ctx context.Context, atespace, name string) error {
	if atespace == "" {
		atespace = "default"
	}
	member := fmt.Sprintf("%s:%s", atespace, name)

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.wsKey(atespace, name))
	pipe.ZRem(ctx, s.wsIndexKey(), member)
	pipe.ZRem(ctx, s.wsAtespaceIndexKey(atespace), name)
	_, err := pipe.Exec(ctx)
	return err
}

// ListWorkspaces lists workspaces for an atespace or across all atespaces.
func (s *Store) ListWorkspaces(ctx context.Context, atespace string) ([]*v1alpha1.Workspace, error) {
	var members []string
	var err error

	if atespace == "" || atespace == "*" {
		members, err = s.client.ZRevRange(ctx, s.wsIndexKey(), 0, -1).Result()
	} else {
		names, nErr := s.client.ZRevRange(ctx, s.wsAtespaceIndexKey(atespace), 0, -1).Result()
		if nErr == nil {
			for _, n := range names {
				members = append(members, fmt.Sprintf("%s:%s", atespace, n))
			}
		}
		err = nErr
	}

	if err != nil {
		return nil, fmt.Errorf("listing workspace index: %w", err)
	}
	if len(members) == 0 {
		return []*v1alpha1.Workspace{}, nil
	}

	keys := make([]string, len(members))
	for i, m := range members {
		parts := strings.SplitN(m, ":", 2)
		if len(parts) == 2 {
			keys[i] = s.wsKey(parts[0], parts[1])
		} else {
			keys[i] = s.wsKey("default", m)
		}
	}

	vals, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("batch fetching workspaces: %w", err)
	}

	workspaces := make([]*v1alpha1.Workspace, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		var w v1alpha1.Workspace
		if err := jsonUnmarshalOpts.Unmarshal([]byte(str), &w); err == nil {
			workspaces = append(workspaces, &w)
		}
	}
	return workspaces, nil
}

// GetWorkspace retrieves a workspace by atespace and name.
func (s *Store) GetWorkspace(ctx context.Context, atespace, name string) (*v1alpha1.Workspace, error) {
	if atespace == "" {
		atespace = "default"
	}
	val, err := s.client.Get(ctx, s.wsKey(atespace, name)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("getting workspace from redis: %w", err)
	}

	var w v1alpha1.Workspace
	if err := jsonUnmarshalOpts.Unmarshal([]byte(val), &w); err != nil {
		return nil, fmt.Errorf("unmarshaling workspace: %w", err)
	}
	return &w, nil
}

// Subscribe joins a Redis Streams consumer group, creating the group (and the
// stream) if needed. A new group starts at the tail of the stream, so it only
// sees events published after it was created; an existing group keeps its
// position and any pending entries.
func (s *Store) Subscribe(ctx context.Context, group, consumer string) (store.Subscription, error) {
	err := s.client.XGroupCreateMkStream(ctx, s.opts.StreamName, group, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, fmt.Errorf("creating consumer group %q: %w", group, err)
	}
	return &subscription{store: s, client: s.client, group: group, consumer: consumer}, nil
}

// subscription reads from a consumer group in batches and hands events out one
// at a time. Delivery is at-least-once: an event stays in the group's pending
// list until Ack is called for it, and entries orphaned by a dead consumer are
// reclaimed by subscribers after ClaimMinIdle — on idle polls and, so failover
// is not coupled to traffic, periodically on busy reads too.
type subscription struct {
	store    *Store
	client   eventStreamClient
	group    string
	consumer string
	pending  []store.TaskEvent
	// lastClaim is when this subscriber last swept the group's pending list
	// for orphaned entries. It throttles the busy-read sweep to one pass per
	// ClaimMinIdle so a loaded stream does not pay an XAUTOCLAIM round-trip
	// on every read.
	lastClaim time.Time
}

// eventStreamClient is the slice of the go-redis client the task-event
// subscription needs. *redis.Client satisfies it; the interface exists so
// tests can stub the transport (scripted XREADGROUP/XAUTOCLAIM results)
// without a live server.
type eventStreamClient interface {
	XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd
	XAutoClaim(ctx context.Context, args *redis.XAutoClaimArgs) *redis.XAutoClaimCmd
}

// claimDue reports whether enough time has passed since this subscriber's
// last pending-list sweep to justify another one.
func (sub *subscription) claimDue() bool {
	return time.Since(sub.lastClaim) >= sub.store.opts.ClaimMinIdle
}

func (sub *subscription) Next(ctx context.Context) (store.TaskEvent, error) {
	for len(sub.pending) == 0 {
		if err := ctx.Err(); err != nil {
			return store.TaskEvent{}, err
		}
		events, err := sub.read(ctx)
		if err != nil {
			return store.TaskEvent{}, err
		}
		sub.pending = events
	}
	ev := sub.pending[0]
	sub.pending = sub.pending[1:]
	return ev, nil
}

// read performs one blocking XREADGROUP call. It returns an empty slice, not an
// error, when the block time elapses without events. Claimed (older) entries
// are delivered before new ones.
func (sub *subscription) read(ctx context.Context) ([]store.TaskEvent, error) {
	streams, err := sub.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    sub.group,
		Consumer: sub.consumer,
		Streams:  []string{sub.store.opts.StreamName, ">"},
		Count:    sub.store.opts.ReadBatchSize,
		Block:    sub.store.opts.ReadBlock,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// No new events: best-effort reclaim of entries a dead consumer
			// claimed but never acknowledged before idling again. Failures
			// are swallowed, not returned: this path just proved the
			// connection healthy, so a failure here is almost certainly an
			// old server without XAUTOCLAIM — returning an error would wedge
			// the worker in a retry loop on every idle poll.
			if claimed, err := sub.claimStale(ctx); err != nil {
				slog.Warn("could not reclaim stale task events (continuing without reclaim)", "error", err)
			} else {
				sub.lastClaim = time.Now()
				return claimed, nil
			}
			return nil, nil
		}
		return nil, fmt.Errorf("reading task events: %w", err)
	}

	var events []store.TaskEvent
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			events = append(events, eventFromMessage(msg))
		}
	}

	// A busy stream never hits the idle path above, so without this a dead
	// consumer's pending entries idle forever while new events keep flowing —
	// failover latency would be unbounded and coupled to traffic. Sweep at
	// most once per ClaimMinIdle: claimed entries are older than anything the
	// ">" read just returned, so they go first.
	if sub.claimDue() {
		if claimed, err := sub.claimStale(ctx); err != nil {
			// Same swallow policy as the idle path: an old server without
			// XAUTOCLAIM must not wedge the worker under load either.
			slog.Warn("could not reclaim stale task events on busy read (continuing)", "error", err)
		} else {
			events = append(claimed, events...)
		}
		sub.lastClaim = time.Now()
	}
	return events, nil
}

// claimStale reclaims pending entries idle longer than ClaimMinIdle — entries a
// consumer claimed but never acknowledged before dying. XREADGROUP with ">"
// never revisits the pending list, so without this a crash between delivery
// and Ack loses the event permanently, and the Close comment's "remain
// claimable" promise would be empty. Claiming resets an entry's idle timer,
// so a slow-but-live consumer is not stolen from while it works.
func (sub *subscription) claimStale(ctx context.Context) ([]store.TaskEvent, error) {
	var events []store.TaskEvent
	start := "0-0"
	for {
		claimed, next, err := sub.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   sub.store.opts.StreamName,
			Group:    sub.group,
			Consumer: sub.consumer,
			MinIdle:  sub.store.opts.ClaimMinIdle,
			Start:    start,
			Count:    sub.store.opts.ReadBatchSize,
		}).Result()
		if err != nil {
			return nil, fmt.Errorf("claiming stale task events: %w", err)
		}
		for _, msg := range claimed {
			events = append(events, eventFromMessage(msg))
		}
		if next == "0-0" {
			break
		}
		start = next
	}
	return events, nil
}

func (sub *subscription) Ack(ctx context.Context, ev store.TaskEvent) error {
	return sub.store.client.XAck(ctx, sub.store.opts.StreamName, sub.group, ev.ID).Err()
}

// Close releases the subscription. The consumer is deliberately left registered
// so that any events it had claimed but not acknowledged remain claimable.
func (sub *subscription) Close() error {
	return nil
}

// eventFromMessage decodes a stream entry. "namespace" is accepted as a legacy
// alias for "atespace".
func eventFromMessage(msg redis.XMessage) store.TaskEvent {
	ev := store.TaskEvent{ID: msg.ID}
	if atespace, ok := msg.Values["atespace"].(string); ok {
		ev.Atespace = atespace
	} else if ns, ok := msg.Values["namespace"].(string); ok {
		ev.Atespace = ns
	}
	if name, ok := msg.Values["name"].(string); ok {
		ev.Name = name
	}
	if action, ok := msg.Values["action"].(string); ok {
		ev.Action = action
	}
	return ev
}

// WatchTask subscribes to status change notifications for a specific task.
func (s *Store) WatchTask(ctx context.Context, atespace, name string) (<-chan *v1alpha1.Task, io.Closer, error) {
	if atespace == "" {
		atespace = "default"
	}
	pubsub := s.client.Subscribe(ctx, s.taskPubSubChannel(atespace, name))
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("subscribing to task watch: %w", err)
	}

	ch := make(chan *v1alpha1.Task, 10)
	go watchForwardLoop(pubsub.Channel(), ch)

	return ch, pubsub, nil
}

// watchForwardLoop decodes Redis pubsub payloads into task updates and
// forwards them to ch until msgCh closes, then closes ch. The send is
// deliberately non-blocking: a consumer that stopped reading (e.g. the
// server's WatchTask returned after a terminal phase) must not pin this
// goroutine forever on a full buffer. This matches MemoryStore.WatchTask,
// which likewise drops to a slow consumer rather than blocking.
func watchForwardLoop(msgCh <-chan *redis.Message, ch chan *v1alpha1.Task) {
	defer close(ch)
	for msg := range msgCh {
		var t v1alpha1.Task
		if err := jsonUnmarshalOpts.Unmarshal([]byte(msg.Payload), &t); err == nil {
			select {
			case ch <- &t:
			default:
			}
		}
	}
}

// Close closes the underlying Redis client connection.
func (s *Store) Close() error {
	return s.client.Close()
}
