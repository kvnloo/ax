package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// The Conditions section header must not carry a leading blank line: every
// other `ax describe task` section prints back-to-back.
func TestWriteConditionsHasNoLeadingBlankLine(t *testing.T) {
	var buf bytes.Buffer
	writeConditions(&buf, []*v1alpha1.Condition{
		{Type: "Ready", Status: "True", Reason: "Running", Message: "all good"},
	})
	out := buf.String()
	if !strings.HasPrefix(out, "Conditions:\n") {
		t.Errorf("conditions output = %q, want it to start with %q", out, "Conditions:\n")
	}
	if strings.Contains(out, "Ready") && !strings.Contains(out, "True") {
		t.Errorf("conditions output dropped fields: %q", out)
	}
}

func TestWriteConditionsEmptyWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	writeConditions(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("writeConditions(nil) wrote %q, want nothing", buf.String())
	}
}
