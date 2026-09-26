package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

// atespaceApplyClient records the atespaces it is asked about, so apply
// routing decisions are observable without a server.
type atespaceApplyClient struct {
	v1alpha1.AXClient
	getAtespaces []string
	updated      []*v1alpha1.Task
}

func (c *atespaceApplyClient) GetTask(ctx context.Context, in *v1alpha1.GetTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	c.getAtespaces = append(c.getAtespaces, in.Atespace)
	return nil, status.Error(codes.NotFound, "no such task")
}

func (c *atespaceApplyClient) UpdateTask(ctx context.Context, in *v1alpha1.UpdateTaskRequest, opts ...grpc.CallOption) (*v1alpha1.Task, error) {
	c.updated = append(c.updated, in.Task)
	return in.Task, nil
}

func applyDocAtespace(t *testing.T, client v1alpha1.AXClient, manifest, atespace string, explicit bool) {
	t.Helper()
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader([]byte(manifest)))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decoding test manifest: %v", err)
	}
	if _, _, _, err := applyDocument(context.Background(), client, &doc, atespace, explicit); err != nil {
		t.Fatalf("applyDocument = %v, want nil", err)
	}
}

// An explicit CLI --atespace wins over the manifest's metadata.atespace
// (kubectl parity). apply never saw the flag at all, so
// `ax apply --atespace=prod -f task.yaml` silently landed in the manifest's
// atespace.
func TestApplyDocumentExplicitAtespaceWins(t *testing.T) {
	fc := &atespaceApplyClient{}
	applyDocAtespace(t, fc,
		"kind: Task\nmetadata:\n  name: demo\n  atespace: dev\nspec: {}\n", "prod", true)
	if len(fc.getAtespaces) != 1 || fc.getAtespaces[0] != "prod" {
		t.Fatalf("GetTask atespaces = %v, want [prod]", fc.getAtespaces)
	}
	if got := fc.updated[0].GetMetadata().GetAtespace(); got != "prod" {
		t.Fatalf("updated task atespace = %q, want prod", got)
	}
}

// Without an explicit flag the manifest's own atespace stands: the default
// "default" must not clobber it.
func TestApplyDocumentManifestAtespaceStands(t *testing.T) {
	fc := &atespaceApplyClient{}
	applyDocAtespace(t, fc,
		"kind: Task\nmetadata:\n  name: demo\n  atespace: dev\nspec: {}\n", "default", false)
	if len(fc.getAtespaces) != 1 || fc.getAtespaces[0] != "dev" {
		t.Fatalf("GetTask atespaces = %v, want [dev]", fc.getAtespaces)
	}
	if got := fc.updated[0].GetMetadata().GetAtespace(); got != "dev" {
		t.Fatalf("updated task atespace = %q, want dev", got)
	}
}

// With neither a flag nor a manifest atespace, the lookup and the update
// agree on the CLI default instead of the lookup going out with "".
func TestApplyDocumentAtespaceDefaults(t *testing.T) {
	fc := &atespaceApplyClient{}
	applyDocAtespace(t, fc,
		"kind: Task\nmetadata:\n  name: demo\nspec: {}\n", "default", false)
	if len(fc.getAtespaces) != 1 || fc.getAtespaces[0] != "default" {
		t.Fatalf("GetTask atespaces = %v, want [default]", fc.getAtespaces)
	}
	if got := fc.updated[0].GetMetadata().GetAtespace(); got != "default" {
		t.Fatalf("updated task atespace = %q, want default", got)
	}
}

// parseGlobalArgs must report whether --atespace was explicitly passed:
// apply needs the distinction for its flag-wins-over-manifest rule.
func TestParseGlobalArgsAtespaceExplicit(t *testing.T) {
	_, _, _, _, _, _, explicit, err := parseGlobalArgs([]string{"get", "tasks", "--atespace=prod"})
	if err != nil || !explicit {
		t.Fatalf("explicit --atespace=prod: explicit=%v err=%v, want true", explicit, err)
	}
	_, _, _, _, _, _, explicit, err = parseGlobalArgs([]string{"-a", "prod", "get", "tasks"})
	if err != nil || !explicit {
		t.Fatalf("explicit -a prod: explicit=%v err=%v, want true", explicit, err)
	}
	_, _, _, _, _, _, explicit, err = parseGlobalArgs([]string{"get", "tasks"})
	if err != nil || explicit {
		t.Fatalf("no atespace flag: explicit=%v err=%v, want false", explicit, err)
	}
}
