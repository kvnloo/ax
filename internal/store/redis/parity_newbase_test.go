package redis_test

// New-base (post-#381) parity tests for the Redis store, driven by a stdlib-only
// in-test RESP fake speaking to the real go-redis client. No network, no Redis.

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/ax/internal/store/redis"
	"github.com/google/ax/pkg/apis/v1alpha1"
	goredis "github.com/redis/go-redis/v9"
)

type fakeRedis struct {
	mu        sync.Mutex
	kv        map[string]string
	zsets     map[string][]string
	setKeys   []string
	zrevCalls [][3]string // key, start, stop
}

func (f *fakeRedis) reply(w *bufio.Writer, s string) {
	fmt.Fprint(w, s+"\r\n")
	w.Flush()
}

func (f *fakeRedis) handleConn(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	var queued [][]string
	inMulti := false
	for {
		n, err := readArrayLen(r)
		if err != nil {
			return
		}
		args := make([]string, 0, n)
		for i := 0; i < n; i++ {
			b, err := readBulk(r)
			if err != nil {
				return
			}
			args = append(args, b)
		}
		if len(args) == 0 {
			continue
		}
		cmd := strings.ToUpper(args[0])
		if inMulti && cmd != "EXEC" && cmd != "DISCARD" {
			queued = append(queued, args)
			f.reply(w, "+QUEUED")
			continue
		}
		switch cmd {
		case "PING":
			f.reply(w, "+PONG")
		case "HELLO":
			// Force the client down to RESP2; this fake only speaks RESP2.
			fmt.Fprint(w, "-ERR unknown command 'hello'\r\n")
			w.Flush()
		case "MULTI":
			inMulti = true
			queued = nil
			f.reply(w, "+OK")
		case "EXEC":
			inMulti = false
			fmt.Fprintf(w, "*%d\r\n", len(queued))
			for _, q := range queued {
				f.execOne(w, q)
			}
			w.Flush()
			queued = nil
		case "DISCARD":
			inMulti = false
			queued = nil
			f.reply(w, "+OK")
		default:
			f.execOne(w, args)
			w.Flush()
		}
	}
}

func (f *fakeRedis) execOne(w *bufio.Writer, args []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "SET":
		f.kv[args[1]] = args[2]
		f.setKeys = append(f.setKeys, args[1])
		fmt.Fprint(w, "+OK\r\n")
	case "GET":
		v, ok := f.kv[args[1]]
		if !ok {
			fmt.Fprint(w, "$-1\r\n")
		} else {
			fmt.Fprintf(w, "$%d\r\n%s\r\n", len(v), v)
		}
	case "DEL":
		n := 0
		for _, k := range args[1:] {
			if _, ok := f.kv[k]; ok {
				delete(f.kv, k)
				n++
			}
		}
		fmt.Fprintf(w, ":%d\r\n", n)
	case "ZADD":
		// ZADD key [NX|XX] [GT|LT] [CH] [INCR] score member [score member ...]
		for i := 1; i+1 < len(args); i += 2 {
			if _, err := strconv.ParseFloat(args[i], 64); err != nil {
				i-- // flag token, re-align
				continue
			}
			f.zsets[args[1]] = append(f.zsets[args[1]], args[i+1])
		}
		fmt.Fprint(w, ":1\r\n")
	case "ZREVRANGE":
		f.zrevCalls = append(f.zrevCalls, [3]string{args[1], args[2], args[3]})
		m := f.zsets[args[1]]
		fmt.Fprintf(w, "*%d\r\n", len(m))
		for _, mm := range m {
			fmt.Fprintf(w, "$%d\r\n%s\r\n", len(mm), mm)
		}
	case "ZREM":
		fmt.Fprint(w, ":0\r\n")
	case "MGET":
		fmt.Fprintf(w, "*%d\r\n", len(args)-1)
		for _, k := range args[1:] {
			if v, ok := f.kv[k]; ok {
				fmt.Fprintf(w, "$%d\r\n%s\r\n", len(v), v)
			} else {
				fmt.Fprint(w, "$-1\r\n")
			}
		}
	case "PUBLISH":
		fmt.Fprint(w, ":0\r\n")
	case "XADD":
		fmt.Fprint(w, "$6\r\n1234-0\r\n")
	default:
		fmt.Fprint(w, "+OK\r\n")
	}
}

func readArrayLen(r *bufio.Reader) (int, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	if len(line) < 2 || line[0] != '*' {
		return 0, fmt.Errorf("expected array, got %q", line)
	}
	return strconv.Atoi(strings.TrimSpace(line[1:]))
}

func readBulk(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 2 || line[0] != '$' {
		return "", fmt.Errorf("expected bulk, got %q", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return "", err
	}
	if n < 0 {
		return "", nil
	}
	buf := make([]byte, n+2)
	for i := 0; i < n+2; {
		m, err := r.Read(buf[i:])
		if err != nil {
			return "", err
		}
		i += m
	}
	return string(buf[:n]), nil
}

func newFakeStore(t *testing.T) (*redis.Store, *fakeRedis) {
	t.Helper()
	f := &fakeRedis{kv: map[string]string{}, zsets: map[string][]string{}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handleConn(c)
		}
	}()
	client := goredis.NewClient(&goredis.Options{Addr: ln.Addr().String(), Protocol: 2})
	t.Cleanup(func() { client.Close() })
	return redis.NewStore(client, redis.Options{}), f
}

func seedTask(t *testing.T, s *redis.Store, name string) {
	t.Helper()
	err := s.SaveTask(context.Background(), &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: name},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A negative offset must be clamped to the head (memory parity); Redis's
// ZRevRange would otherwise count from the tail and return an empty page.
func TestRedisListTasksNegativeOffsetClamped(t *testing.T) {
	s, f := newFakeStore(t)
	seedTask(t, s, "a")
	seedTask(t, s, "b")
	seedTask(t, s, "c")
	f.zrevCalls = nil
	_, err := s.ListTasks(context.Background(), "", 50, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.zrevCalls) == 0 {
		t.Fatal("no ZREVRANGE recorded")
	}
	if f.zrevCalls[0][1] != "0" {
		t.Fatalf("ZREVRANGE start = %q, want clamped \"0\"", f.zrevCalls[0][1])
	}
}

// UpdateTaskStatus must write the same canonical key GetTask reads.
// GetTask folds "" -> "default"; the write must agree (no orphan ax:task::name).
func TestRedisUpdateTaskStatusEmptyAtespaceKey(t *testing.T) {
	s, f := newFakeStore(t)
	seedTask(t, s, "t1") // SaveTask normalizes "" -> "default"
	f.setKeys = nil
	err := s.UpdateTaskStatus(context.Background(), "", "t1", &v1alpha1.TaskStatus{Phase: "Running"})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range f.setKeys {
		if k == "ax:task::t1" {
			t.Fatalf("orphaned key written: %q (want ax:task:default:t1)", k)
		}
	}
	found := false
	for _, k := range f.setKeys {
		if k == "ax:task:default:t1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("canonical key not written; SET keys: %v", f.setKeys)
	}
}
