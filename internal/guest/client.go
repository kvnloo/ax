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

package guest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	ateenvv1alpha "github.com/agent-substrate/env/proto/ateenv/v1alpha"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Client interacts with the in-actor guest daemon.
type Client struct {
	grpcConn   *grpc.ClientConn
	process    ateenvv1alpha.ProcessServiceClient
	filesystem ateenvv1alpha.FileSystemServiceClient
}

// Dial connects to the guest daemon at the specified target (e.g., "127.0.0.1:80").
func Dial(target string) (*Client, error) {
	return DialTarget(target, "")
}

// DialTarget connects to the guest daemon at the specified target, optionally injecting
// the 'ate-target-actor' metadata header on all calls (required when routing via atenet-router).
func DialTarget(target string, targetActor string) (*Client, error) {
	target = strings.TrimPrefix(target, "http://")
	target = strings.TrimPrefix(target, "https://")

	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if targetActor != "" {
		dialOpts = append(dialOpts,
			grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				ctx = metadata.AppendToOutgoingContext(ctx, "ate-target-actor", targetActor)
				return invoker(ctx, method, req, reply, cc, opts...)
			}),
			grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				ctx = metadata.AppendToOutgoingContext(ctx, "ate-target-actor", targetActor)
				return streamer(ctx, desc, cc, method, opts...)
			}),
		)
	}

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to guest at %s: %w", target, err)
	}

	return &Client{
		grpcConn:   conn,
		process:    ateenvv1alpha.NewProcessServiceClient(conn),
		filesystem: ateenvv1alpha.NewFileSystemServiceClient(conn),
	}, nil
}

// Close terminates the underlying gRPC client connection.
func (c *Client) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}

// WaitReady blocks until the underlying gRPC connection reaches READY or ctx
// expires. gRPC dials lazily, so without this the first real RPC on the
// direct ssh path is the Exec — and a half-open direct path (TCP accepted, no
// gRPC server: stale worker IP, intercepting middlebox) would surface only
// there, after the atenet-router fallback was already skipped. Callers must
// run this before any command starts: falling back at this point cannot
// double-execute a side-effecting command.
func (c *Client) WaitReady(ctx context.Context) error {
	if c.grpcConn == nil {
		return errors.New("guest: no connection")
	}
	c.grpcConn.Connect()
	for {
		st := c.grpcConn.GetState()
		if st == connectivity.Ready {
			return nil
		}
		if st == connectivity.Shutdown {
			// A closed connection never changes state again; without this,
			// WaitForStateChange below would block until ctx expires even
			// when ctx has no deadline, contradicting this function's
			// contract.
			return errors.New("guest: connection is closed")
		}
		if !c.grpcConn.WaitForStateChange(ctx, st) {
			return fmt.Errorf("guest: connection not ready: %w", ctx.Err())
		}
	}
}

// ExecOptions holds parameters for running a command in the task actor.
type ExecOptions struct {
	Command []string
	Cwd     string
	Env     map[string]string
	Stdout  io.Writer
	Stderr  io.Writer
}

// signalKill best-effort delivers SIGKILL to the process group of a remote
// process Exec started but can no longer observe. It runs on a fresh context
// so it still works when the caller's ctx is already done, and its own
// errors are swallowed: the process may already be gone.
func (c *Client) signalKill(processID string) {
	if processID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = c.process.SignalProcess(ctx, &ateenvv1alpha.SignalProcessRequest{
		ProcessId: processID,
		Signal:    ateenvv1alpha.Signal_SIGNAL_KILL,
	})
}

// Exec runs a command inside the task container and streams stdout/stderr until completion.
// It returns the process exit code.
func (c *Client) Exec(ctx context.Context, opts ExecOptions) (int, error) {
	if len(opts.Command) == 0 {
		return 1, errors.New("exec: command cannot be empty")
	}

	proc, err := c.process.StartProcess(ctx, &ateenvv1alpha.StartProcessRequest{
		Command: opts.Command,
		Cwd:     opts.Cwd,
		Env:     opts.Env,
	})
	if err != nil {
		return 1, fmt.Errorf("starting process: %w", err)
	}

	pid := proc.GetProcessId()
	stream, err := c.process.StreamProcessOutput(ctx, &ateenvv1alpha.StreamProcessOutputRequest{
		ProcessId: pid,
		Follow:    true,
	})
	if err != nil {
		// Never got a working stream: the process would run on unobserved.
		c.signalKill(pid)
		return 1, fmt.Errorf("streaming process output: %w", err)
	}

	// With Follow set, the stream ends with the final process state.
	for {
		out, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Stream broke before the exit was observed: kill rather than
			// orphan a running process nobody will reap.
			c.signalKill(pid)
			return 1, fmt.Errorf("receiving output stream: %w", err)
		}
		if exit := out.GetExit(); exit != nil {
			return int(exit.GetExitCode()), nil
		}
		if data := out.GetStdout(); len(data) > 0 && opts.Stdout != nil {
			_, _ = opts.Stdout.Write(data)
		}
		if data := out.GetStderr(); len(data) > 0 && opts.Stderr != nil {
			_, _ = opts.Stderr.Write(data)
		}
	}

	// The stream closed without reporting an exit; fall back to polling.
	for {
		proc, err := c.process.GetProcess(ctx, &ateenvv1alpha.GetProcessRequest{ProcessId: pid})
		if err != nil {
			c.signalKill(pid)
			return 1, fmt.Errorf("getting process state: %w", err)
		}
		if proc.GetState() != ateenvv1alpha.ProcessState_PROCESS_STATE_RUNNING {
			return int(proc.GetExitCode()), nil
		}
		select {
		case <-ctx.Done():
			// Caller gave up waiting: do not leave the process running
			// unobserved behind.
			c.signalKill(pid)
			return 1, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
