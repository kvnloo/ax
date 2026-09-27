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

package controller

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

func digestTestTask() *v1alpha1.Task {
	return &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Name: "t", Atespace: "default"},
		Spec: &v1alpha1.TaskSpec{
			Image: "img",
			Env:   []*v1alpha1.EnvVar{{Name: "FOO", Value: "bar"}},
		},
		Status: &v1alpha1.TaskStatus{Phase: "Running", WorkerIp: "10.0.0.1"},
	}
}

func digestTestEnv(task *v1alpha1.Task) map[string]string {
	return map[string]string{
		"FOO":          "bar",
		"AX_TASK_YAML": "task-yaml-including-status",
	}
}

// Status-only differences must not change the template name.
func TestTemplateEnvForDigest_StableAcrossStatusChanges(t *testing.T) {
	a := digestTestTask()
	b := digestTestTask()
	b.Status.Phase = "Suspended"
	b.Status.WorkerIp = ""
	b.Status.Id = "task-t-123"

	nameA := taskTemplateName("t", "img", templateEnvForDigest(a, digestTestEnv(a)))
	nameB := taskTemplateName("t", "img", templateEnvForDigest(b, digestTestEnv(b)))
	if nameA != nameB {
		t.Errorf("template name changed on status-only difference: %q vs %q", nameA, nameB)
	}
}

// Spec differences must still change the template name.
func TestTemplateEnvForDigest_ChangesOnSpecChange(t *testing.T) {
	a := digestTestTask()
	b := digestTestTask()
	b.Spec.Env = []*v1alpha1.EnvVar{{Name: "FOO", Value: "baz"}}

	nameA := taskTemplateName("t", "img", templateEnvForDigest(a, digestTestEnv(a)))
	nameB := taskTemplateName("t", "img", templateEnvForDigest(b, digestTestEnv(b)))
	if nameA == nameB {
		t.Errorf("template name did not change on spec change: both %q", nameA)
	}
}

func TestTemplateEnvForDigest_ChangesOnImageChange(t *testing.T) {
	a := digestTestTask()
	nameA := taskTemplateName("t", "img", templateEnvForDigest(a, digestTestEnv(a)))
	nameB := taskTemplateName("t", "img2", templateEnvForDigest(a, digestTestEnv(a)))
	if nameA == nameB {
		t.Errorf("template name did not change on image change: both %q", nameA)
	}
}
