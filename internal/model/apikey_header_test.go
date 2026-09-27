// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on a "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/ax/internal/model"
)

// The Gemini API key must travel in the x-goog-api-key header, never in the
// URL query string: query strings are written to HTTP access logs by proxies
// and servers, while the header is the documented credential channel.
func TestClient_GeminiAPIKeyViaHeader(t *testing.T) {
	const apiKey = "test-api-key-123"
	var gotHeader string
	var gotQueryKey bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-goog-api-key")
		gotQueryKey = r.URL.Query().Has("key")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"candidates": []map[string]interface{}{
				{"content": map[string]interface{}{
					"parts": []map[string]string{{"text": "hi"}},
				}},
			},
		})
	}))
	defer ts.Close()

	client := model.NewClient(
		model.Config{
			Provider: "google",
			Model:    "gemini-3.8-flash",
			BaseURL:  ts.URL,
			APIKey:   apiKey,
		},
		model.WithHTTPClient(ts.Client()),
	)

	if _, err := client.Generate(context.Background(), &model.GenerateRequest{Prompt: "hi"}); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if gotQueryKey {
		t.Errorf("request URL carries ?key=: API key leaked into the query string")
	}
	if gotHeader != apiKey {
		t.Errorf("x-goog-api-key header = %q; want the configured API key", gotHeader)
	}
}
