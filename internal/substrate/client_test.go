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

package substrate_test

import (
	"context"
	"net"
	"testing"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"github.com/google/ax/internal/substrate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// fakeControlServer serves canned pages of ActorTemplates.
type fakeControlServer struct {
	ateapipb.UnimplementedControlServer
	pages [][]*ateapipb.ActorTemplate
}

func (f *fakeControlServer) ListActorTemplates(_ context.Context, req *ateapipb.ListActorTemplatesRequest) (*ateapipb.ListActorTemplatesResponse, error) {
	idx := 0
	if req.GetPageToken() != "" {
		idx = 1
	}
	resp := &ateapipb.ListActorTemplatesResponse{ActorTemplates: f.pages[idx]}
	if idx+1 < len(f.pages) {
		resp.NextPageToken = "next"
	}
	return resp, nil
}

func tmpl(name string) *ateapipb.ActorTemplate {
	return &ateapipb.ActorTemplate{Metadata: &ateapipb.ResourceMetadata{Name: name}}
}

func newBufconnClient(t *testing.T, srv ateapipb.ControlServer) *substrate.Client {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	ateapipb.RegisterControlServer(gs, srv)
	go gs.Serve(lis)
	t.Cleanup(gs.Stop)

	c, err := substrate.NewClient("bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Template garbage collection (deleteTaskTemplates) matches every template
// belonging to a task by name pattern. If ListActorTemplates stopped at the
// first page, templates on later pages would never match and would leak.
func TestListActorTemplatesFollowsPagination(t *testing.T) {
	c := newBufconnClient(t, &fakeControlServer{pages: [][]*ateapipb.ActorTemplate{
		{tmpl("t1"), tmpl("t2")},
		{tmpl("t3")},
	}})

	got, err := c.ListActorTemplates(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListActorTemplates: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d templates, want 3 (every page)", len(got))
	}
	for i, want := range []string{"t1", "t2", "t3"} {
		if got[i].GetMetadata().GetName() != want {
			t.Fatalf("template %d = %q, want %q", i, got[i].GetMetadata().GetName(), want)
		}
	}
}
