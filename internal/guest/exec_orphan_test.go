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
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	ateenvv1alpha "github.com/agent-substrate/env/proto/ateenv/v1alpha"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// execFakeServer is a controllable ProcessService: it starts processes with a
// fixed id and records every SignalProcess call so tests can assert Exec
// cleans up (or deliberately does not clean up) the remote process.
type execFakeServer struct {
	ateenvv1alpha.UnimplementedProcessServiceServer

	mu        sync.Mutex
	signals   []ateenvv1alpha.Signal
	signalPID []string

	// streamMsgs are sent on StreamProcessOutput before streamErr/EOF.
	streamMsgs []*ateenvv1alpha.ProcessOutput
	streamErr  error
	// getProcErr, when non-nil, is returned by GetProcess.
	getProcErr error
}

func (f *execFakeServer) StartProcess(ctx context.Context, req *ateenvv1alpha.StartProcessRequest) (*ateenvv1alpha.Process, error) {
	return &ateenvv1alpha.Process{ProcessId: "p1", State: ateenvv1alpha.ProcessState_PROCESS_STATE_RUNNING}, nil
}

func (f *execFakeServer) StreamProcessOutput(req *ateenvv1alpha.StreamProcessOutputRequest, stream grpc.ServerStreamingServer[ateenvv1alpha.ProcessOutput]) error {
	for _, m := range f.streamMsgs {
		if err := stream.Send(m); err != nil {
			return err
		}
	}
	return f.streamErr
}

func (f *execFakeServer) GetProcess(ctx context.Context, req *ateenvv1alpha.GetProcessRequest) (*ateenvv1alpha.Process, error) {
	if f.getProcErr != nil {
		return nil, f.getProcErr
	}
	return &ateenvv1alpha.Process{
		ProcessId: req.GetProcessId(),
		State:     ateenvv1alpha.ProcessState_PROCESS_STATE_EXITED,
		ExitCode:  3,
	}, nil
}

func (f *execFakeServer) SignalProcess(ctx context.Context, req *ateenvv1alpha.SignalProcessRequest) (*ateenvv1alpha.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, req.GetSignal())
	f.signalPID = append(f.signalPID, req.GetProcessId())
	return &ateenvv1alpha.Process{ProcessId: req.GetProcessId()}, nil
}

func (f *execFakeServer) killSignals() []ateenvv1alpha.Signal {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ateenvv1alpha.Signal(nil), f.signals...)
}

// serveExecFake starts the fake on a loopback listener and returns a client.
func serveExecFake(t *testing.T, fake *execFakeServer) *Client {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	ateenvv1alpha.RegisterProcessServiceServer(srv, fake)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	c, err := Dial(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func stdoutMsg(data string) *ateenvv1alpha.ProcessOutput {
	return &ateenvv1alpha.ProcessOutput{Output: &ateenvv1alpha.ProcessOutput_Stdout{Stdout: []byte(data)}}
}

func exitMsg(code int32) *ateenvv1alpha.ProcessOutput {
	return &ateenvv1alpha.ProcessOutput{Output: &ateenvv1alpha.ProcessOutput_Exit{Exit: &ateenvv1alpha.Process{
		ProcessId: "p1",
		State:     ateenvv1alpha.ProcessState_PROCESS_STATE_EXITED,
		ExitCode:  code,
	}}}
}

// TestExecRecvErrorKillsProcess: the stream breaks mid-run before the exit
// message. Exec must report the error and SIGKILL the orphan, not return
// while it keeps running.
func TestExecRecvErrorKillsProcess(t *testing.T) {
	fake := &execFakeServer{
		streamMsgs: []*ateenvv1alpha.ProcessOutput{stdoutMsg("hello")},
		streamErr:  status.Error(codes.Internal, "boom"),
	}
	c := serveExecFake(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	_, err := c.Exec(ctx, ExecOptions{Command: []string{"echo", "hi"}, Stdout: &out})
	if err == nil {
		t.Fatal("Exec with a broken stream: want error")
	}
	if out.String() != "hello" {
		t.Fatalf("output delivered before the break must survive, got %q", out.String())
	}
	sigs := fake.killSignals()
	if len(sigs) != 1 || sigs[0] != ateenvv1alpha.Signal_SIGNAL_KILL {
		t.Fatalf("want exactly one SIGKILL, got %v", sigs)
	}
}

// TestExecGetProcessErrorKillsProcess: the Follow stream ends early (EOF, no
// exit message) and the polling fallback cannot read the process state.
// Exec must SIGKILL rather than return while the process may still run.
func TestExecGetProcessErrorKillsProcess(t *testing.T) {
	fake := &execFakeServer{getProcErr: status.Error(codes.Unavailable, "boom")}
	c := serveExecFake(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.Exec(ctx, ExecOptions{Command: []string{"true"}})
	if err == nil {
		t.Fatal("Exec with failing GetProcess: want error")
	}
	sigs := fake.killSignals()
	if len(sigs) != 1 || sigs[0] != ateenvv1alpha.Signal_SIGNAL_KILL {
		t.Fatalf("want exactly one SIGKILL, got %v", sigs)
	}
}

// TestExecCleanExitDoesNotSignal: a normal run ending with the exit message
// must not send any signal — the kill path is only for abandoned processes.
func TestExecCleanExitDoesNotSignal(t *testing.T) {
	fake := &execFakeServer{
		streamMsgs: []*ateenvv1alpha.ProcessOutput{stdoutMsg("out"), exitMsg(7)},
	}
	c := serveExecFake(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	code, err := c.Exec(ctx, ExecOptions{Command: []string{"exit", "7"}, Stdout: &out})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if out.String() != "out" {
		t.Fatalf("stdout = %q, want %q", out.String(), "out")
	}
	if sigs := fake.killSignals(); len(sigs) != 0 {
		t.Fatalf("clean exit sent signals %v, want none", sigs)
	}
}
