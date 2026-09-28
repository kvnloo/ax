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

package lock_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/ax/internal/lock"
	"github.com/redis/go-redis/v9"
)

// fakeEntry is one key in the fake Redis: a value plus an optional deadline.
type fakeEntry struct {
	val string
	exp time.Time // zero = no expiry
}

// fakeRedis is an in-process stand-in for a Redis server, wired in as a
// go-redis Hook so no real server is needed. It implements just enough of
// the command surface RedisLocker uses: SET (NX/XX/PX/EX), GET, DEL,
// PUBLISH, and the EVALSHA/EVAL scripts (release + renew), with real
// TTL-expiry semantics.
type fakeRedis struct {
	mu  sync.Mutex
	kv  map[string]fakeEntry
	pub map[string]int
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{kv: map[string]fakeEntry{}, pub: map[string]int{}}
}

func (f *fakeRedis) expiredLocked(key string) bool {
	e, ok := f.kv[key]
	if !ok {
		return true
	}
	if !e.exp.IsZero() && !time.Now().Before(e.exp) {
		delete(f.kv, key)
		return true
	}
	return false
}

func (f *fakeRedis) get(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.expiredLocked(key) {
		return "", false
	}
	return f.kv[key].val, true
}

// setArgs parses "set key val [nx|xx] [px|ex ttl]".
func setArgs(args []interface{}) (key, val string, nx, xx bool, ttl time.Duration) {
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	key, _ = args[1].(string)
	val, _ = args[2].(string)
	for i := 3; i < len(args); i++ {
		switch strings.ToLower(str(args[i])) {
		case "nx":
			nx = true
		case "xx":
			xx = true
		case "px":
			i++
			if ms, err := strconv.ParseInt(str(args[i]), 10, 64); err == nil {
				ttl = time.Duration(ms) * time.Millisecond
			}
		case "ex":
			i++
			if s, err := strconv.ParseInt(str(args[i]), 10, 64); err == nil {
				ttl = time.Duration(s) * time.Second
			}
		}
	}
	return key, val, nx, xx, ttl
}

// setCmd/intCmd/strCmd/anyCmd set the result on the concrete command type.
func setCmd(cmd redis.Cmder, ok bool) {
	if c, is := cmd.(*redis.BoolCmd); is {
		c.SetVal(ok)
	}
}

func intCmd(cmd redis.Cmder, n int64) {
	if c, is := cmd.(*redis.IntCmd); is {
		c.SetVal(n)
	}
}

func strCmd(cmd redis.Cmder, s string, err error) {
	if c, is := cmd.(*redis.StringCmd); is {
		c.SetVal(s)
		c.SetErr(err)
	}
}

func anyCmd(cmd redis.Cmder, v interface{}) {
	if c, is := cmd.(*redis.Cmd); is {
		c.SetVal(v)
	}
}

func (f *fakeRedis) DialHook(next redis.DialHook) redis.DialHook { return next }

func (f *fakeRedis) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		args := cmd.Args()
		switch cmd.Name() {
		case "set":
			key, val, nx, xx, ttl := setArgs(args)
			missing := f.expiredLocked(key)
			setOK := true
			if nx && !missing {
				setOK = false
			}
			if xx && missing {
				setOK = false
			}
			e := fakeEntry{val: val}
			if ttl > 0 {
				e.exp = time.Now().Add(ttl)
			}
			if setOK {
				f.kv[key] = e
			}
			setCmd(cmd, setOK)
			return nil
		case "get":
			key, _ := args[1].(string)
			if f.expiredLocked(key) {
				strCmd(cmd, "", redis.Nil)
				return nil
			}
			strCmd(cmd, f.kv[key].val, nil)
			return nil
		case "del":
			n := int64(0)
			for _, a := range args[1:] {
				if key, ok := a.(string); ok && !f.expiredLocked(key) {
					delete(f.kv, key)
					n++
				}
			}
			intCmd(cmd, n)
			return nil
		case "publish":
			ch, _ := args[1].(string)
			f.pub[ch]++
			intCmd(cmd, 1)
			return nil
		case "evalsha", "eval":
			// args: evalsha <sha> <numkeys> <keys...> <argv...>
			str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
			nkeys, _ := strconv.Atoi(str(args[2]))
			keys := make([]string, 0, nkeys)
			for _, a := range args[3 : 3+nkeys] {
				keys = append(keys, str(a))
			}
			argv := args[3+nkeys:]
			if nkeys == 2 {
				// releaseAndNotifyScript: get==token ? del+publish : 0
				v := int64(0)
				if !f.expiredLocked(keys[0]) && f.kv[keys[0]].val == str(argv[0]) {
					delete(f.kv, keys[0])
					f.pub[keys[1]]++
					v = 1
				}
				anyCmd(cmd, v)
				return nil
			}
			if nkeys == 1 {
				// renewScript: get==token ? pexpire : 0
				v := int64(0)
				if !f.expiredLocked(keys[0]) && f.kv[keys[0]].val == str(argv[0]) {
					if ms, err := strconv.ParseInt(str(argv[1]), 10, 64); err == nil {
						e := f.kv[keys[0]]
						e.exp = time.Now().Add(time.Duration(ms) * time.Millisecond)
						f.kv[keys[0]] = e
					}
					v = 1
				}
				anyCmd(cmd, v)
				return nil
			}
			anyCmd(cmd, int64(0))
			return nil
		}
		return next(ctx, cmd)
	}
}

func (f *fakeRedis) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func newTestLocker(fr *fakeRedis, ttl time.Duration) (*redis.Client, *lock.RedisLocker) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // never dialed: hook intercepts everything
	client.AddHook(fr)
	return client, lock.NewRedisLocker(client, lock.RedisLockerOptions{TTL: ttl})
}

// TestRedisLocker_RenewsTTLWhileHeld proves the lock's TTL is kept alive
// for as long as it is held. The server holds task locks across full
// substrate reconciles (actor provisioning, template creation, workspace
// setup waits), which routinely exceed the TTL; without renewal the key
// expires mid-operation and a second caller acquires the same lock.
func TestRedisLocker_RenewsTTLWhileHeld(t *testing.T) {
	fr := newFakeRedis()
	client, locker := newTestLocker(fr, 200*time.Millisecond)
	defer client.Close()
	ctx := context.Background()

	unlock, err := locker.Lock(ctx, "task", "default", "renew-me")
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer unlock()

	key := lock.Key("task", "default", "renew-me")
	holderToken, ok := fr.get(key)
	if !ok {
		t.Fatalf("lock key %q missing right after acquire", key)
	}

	time.Sleep(600 * time.Millisecond) // 3x TTL: without renewal the key is long gone

	// A contender must NOT be able to take the key while it is held.
	acquired, err := client.SetNX(ctx, key, "contender-token", time.Minute).Result()
	if err != nil {
		t.Fatalf("contender SetNX: %v", err)
	}
	if acquired {
		t.Fatalf("lock TTL expired while still held: a second caller acquired the same lock")
	}
	if tok, _ := fr.get(key); tok != holderToken {
		t.Fatalf("lock changed hands while held")
	}
}

// TestRedisLocker_UnlockStopsRenewal guards the other side: after unlock the
// key must be gone and must stay gone (no renewal resurrecting it), so the
// next waiter can acquire.
func TestRedisLocker_UnlockStopsRenewal(t *testing.T) {
	fr := newFakeRedis()
	client, locker := newTestLocker(fr, 200*time.Millisecond)
	defer client.Close()
	ctx := context.Background()

	unlock, err := locker.Lock(ctx, "task", "default", "release-me")
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	key := lock.Key("task", "default", "release-me")
	unlock()

	time.Sleep(400 * time.Millisecond) // > TTL: any stray renewal would have fired by now
	if _, ok := fr.get(key); ok {
		t.Fatalf("lock key %q still present after unlock", key)
	}
	acquired, err := client.SetNX(ctx, key, "next-holder", time.Minute).Result()
	if err != nil {
		t.Fatalf("re-acquire SetNX: %v", err)
	}
	if !acquired {
		t.Fatalf("could not re-acquire lock after unlock")
	}
}
