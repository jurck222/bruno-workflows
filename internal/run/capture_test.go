package run

import (
	"net/http"
	"strings"
	"testing"
)

func resp(body string) *Response {
	return &Response{
		Status: 201,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   body,
	}
}

func TestCapture(t *testing.T) {
	r := resp(`{"data":{"id":"abc123"},"items":[{"id":"i0"},{"id":"i1"}]}`)

	cases := []struct {
		expr string
		want string
	}{
		{"status", "201"},
		{"header.Content-Type", "application/json"},
		{"header.content-type", "application/json"},
		{"body.data.id", "abc123"},
		{"body.items.0.id", "i0"},
		{"body.items.1.id", "i1"},
	}
	for _, c := range cases {
		got, err := Capture(c.expr, r)
		if err != nil {
			t.Errorf("Capture(%q): %v", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("Capture(%q) = %q, want %q", c.expr, got, c.want)
		}
	}
}

func TestCaptureErrors(t *testing.T) {
	r := resp(`{"data":{"id":"abc123"},"items":[{"id":"i0"}]}`)

	cases := []struct {
		expr    string
		wantErr string
	}{
		{"body.missing", `no key "missing"`},
		{"body.items.5", "out of range"},
		{"body.data", "object or array"},
	}
	for _, c := range cases {
		_, err := Capture(c.expr, r)
		if err == nil {
			t.Fatalf("Capture(%q) should error", c.expr)
		}
		if !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("Capture(%q) error = %q, want to contain %q", c.expr, err.Error(), c.wantErr)
		}
	}
}

func TestInterpolate(t *testing.T) {
	vars := map[string]string{"AUTH_TOKEN": "tok123"}
	dotenv := map[string]string{"FOO": "bar"}

	got, err := Interpolate("Bearer {{AUTH_TOKEN}}", vars, dotenv)
	if err != nil || got != "Bearer tok123" {
		t.Fatalf("got %q, %v", got, err)
	}

	got, err = Interpolate("{{updater.units-api-url}}/x", map[string]string{"updater.units-api-url": "https://h"}, nil)
	if err != nil || got != "https://h/x" {
		t.Fatalf("got %q, %v", got, err)
	}

	got, err = Interpolate("{{process.env.FOO}}", nil, dotenv)
	if err != nil || got != "bar" {
		t.Fatalf("got %q, %v", got, err)
	}

	_, err = Interpolate("{{MISSING}}", vars, nil)
	if err == nil {
		t.Fatal("want error for unresolved var")
	}
}
