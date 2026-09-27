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
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
)

// fakeRedis is a minimal in-test RESP2 server: just enough surface (GET, SET,
// MULTI/EXEC, PUBLISH) for the store methods under test. It exists because
// this environment has no redis-server binary and adding a test-only redis
// dependency is not warranted for a 3-line parity fix. Keys are tracked so
// the test can assert exactly which key a write landed on.
type fakeRedis struct {
	t    *testing.T
	ln   net.Listener
	mu   sync.Mutex
	data map[string]string
	sets []string // keys SET (in order), including inside MULTI/EXEC
}

func newFakeRedis(t *testing.T) *fakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRedis{t: t, ln: ln, data: map[string]string{}}
	go f.serve()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeRedis) addr() string { return f.ln.Addr().String() }

func (f *fakeRedis) set(key, val string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = val
}

func (f *fakeRedis) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[key]
	return ok
}

func (f *fakeRedis) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(c)
	}
}

func (f *fakeRedis) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	var multi bool
	var queued [][]string
	flush := func(cmds [][]string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, q := range cmds {
			switch strings.ToUpper(q[0]) {
			case "SET":
				f.data[q[1]] = q[2]
				f.sets = append(f.sets, q[1])
			case "DEL":
				delete(f.data, q[1])
			}
		}
	}
	for {
		args, err := readCommand(r)
		if err != nil {
			return
		}
		if len(args) == 0 {
			continue
		}
		cmd := strings.ToUpper(args[0])
		switch {
		case multi && cmd != "EXEC" && cmd != "DISCARD":
			queued = append(queued, args)
			fmt.Fprint(c, "+QUEUED\r\n")
		case cmd == "MULTI":
			multi = true
			fmt.Fprint(c, "+OK\r\n")
		case cmd == "EXEC":
			multi = false
			cmds := queued
			queued = nil
			flush(cmds)
			n := len(cmds)
			fmt.Fprintf(c, "*%d\r\n", n)
			for _, q := range cmds {
				switch strings.ToUpper(q[0]) {
				case "PUBLISH":
					fmt.Fprint(c, ":0\r\n")
				default:
					fmt.Fprint(c, "+OK\r\n")
				}
			}
		case cmd == "GET":
			f.mu.Lock()
			v, ok := f.data[args[1]]
			f.mu.Unlock()
			if !ok {
				fmt.Fprint(c, "$-1\r\n")
			} else {
				fmt.Fprintf(c, "$%d\r\n%s\r\n", len(v), v)
			}
		case cmd == "SET":
			f.set(args[1], args[2])
			fmt.Fprint(c, "+OK\r\n")
		case cmd == "HELLO":
			// Signal "unsupported" so go-redis falls back to RESP2.
			fmt.Fprint(c, "-ERR unknown command 'hello'\r\n")
		case cmd == "PUBLISH":
			fmt.Fprint(c, ":0\r\n")
		default:
			// Client handshakes (CLIENT SETINFO etc.) — not under test.
			fmt.Fprint(c, "+OK\r\n")
		}
	}
}

// readCommand reads one RESP2 multibulk command.
func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "*") {
		return nil, io.ErrUnexpectedEOF
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(hdr, "$") {
			return nil, io.ErrUnexpectedEOF
		}
		l, err := strconv.Atoi(strings.TrimSpace(hdr[1:]))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, l+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:l]))
	}
	return args, nil
}

// TestUpdateTaskStatusEmptyAtespaceWritesNormalizedKey pins the store contract:
// an empty atespace means "default" everywhere, on both backends. GetTask,
// MarkTaskDeleting and DeleteTask already normalize; UpdateTaskStatus must
// write the same key it read, or the status lands on an orphaned key and the
// next read returns stale data.
func TestUpdateTaskStatusEmptyAtespaceWritesNormalizedKey(t *testing.T) {
	ctx := context.Background()
	f := newFakeRedis(t)
	client := redis.NewClient(&redis.Options{Addr: f.addr()})
	defer client.Close()
	st := NewStore(client, Options{})

	seed := &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t1", Atespace: "default"},
		Status:   &v1alpha1.TaskStatus{Phase: "Running"},
	}
	raw, err := protojson.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	f.set("ax:task:default:t1", string(raw))

	if err := st.UpdateTaskStatus(ctx, "", "t1", &v1alpha1.TaskStatus{Phase: "Succeeded"}); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	got, err := st.GetTask(ctx, "default", "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.GetStatus().GetPhase() != "Succeeded" {
		t.Fatalf("canonical key phase = %q, want Succeeded (status update was lost)", got.GetStatus().GetPhase())
	}
	if f.has("ax:task::t1") {
		t.Fatal("orphaned key ax:task::t1 exists: write went to the raw atespace instead of default")
	}
}
