package memory_test

import (
	"context"
	"testing"

	"github.com/google/ax/internal/store/memory"
	v1alpha1 "github.com/google/ax/pkg/apis/v1alpha1"
)

// Saving a resource with nil Metadata must return an error, not panic.
// The Redis store allocates the metadata struct; the memory store must agree.
func TestSaveGatewayNilMetadataReturnsError(t *testing.T) {
	s := memory.NewStore()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SaveGateway(nil Metadata) panicked: %v", r)
		}
	}()
	err := s.SaveGateway(context.Background(), &v1alpha1.Gateway{})
	if err == nil {
		t.Fatal("expected error for missing gateway name, got nil")
	}
}

func TestSaveModelNilMetadataReturnsError(t *testing.T) {
	s := memory.NewStore()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SaveModel(nil Metadata) panicked: %v", r)
		}
	}()
	err := s.SaveModel(context.Background(), &v1alpha1.Model{})
	if err == nil {
		t.Fatal("expected error for missing model name, got nil")
	}
}

func TestSaveWorkspaceNilMetadataReturnsError(t *testing.T) {
	s := memory.NewStore()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SaveWorkspace(nil Metadata) panicked: %v", r)
		}
	}()
	err := s.SaveWorkspace(context.Background(), &v1alpha1.Workspace{})
	if err == nil {
		t.Fatal("expected error for missing workspace name, got nil")
	}
}

// The nil-metadata input must be usable afterward: the store must not have
// mutated the caller's object with a half-written key (parity with Redis,
// which normalizes Metadata.Atespace before writing).
func TestSaveGatewayNilMetadataThenNamedSave(t *testing.T) {
	s := memory.NewStore()
	gw := &v1alpha1.Gateway{}
	if err := s.SaveGateway(context.Background(), gw); err == nil {
		t.Fatal("expected error for missing gateway name")
	}
	gw.Metadata.Name = "gw1"
	if err := s.SaveGateway(context.Background(), gw); err != nil {
		t.Fatalf("named save failed: %v", err)
	}
	got, err := s.GetGateway(context.Background(), "", "gw1")
	if err != nil {
		t.Fatalf("GetGateway: %v", err)
	}
	if got.Metadata.Atespace != "default" {
		t.Fatalf("atespace = %q, want default", got.Metadata.Atespace)
	}
}
