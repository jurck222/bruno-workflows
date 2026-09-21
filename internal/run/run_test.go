package run

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thecalda/brw/internal/collection"
	"github.com/thecalda/brw/internal/workflow"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"})
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"ok":true,"auth":"` + gotAuth + `"}`))
	})
	return httptest.NewServer(mux)
}

func twoStepWorkflow(loginReq, meReq string) *workflow.Workflow {
	return &workflow.Workflow{
		Name: "two-step",
		Steps: []workflow.Step{
			{
				Request: loginReq,
				Name:    "Login",
				Capture: map[string]string{"AUTH_TOKEN": "body.access_token"},
			},
			{
				Request: meReq,
				Name:    "Me",
			},
		},
	}
}

func TestRunPropagatesCapturedAuthBru(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	col, err := collection.Open("../../testdata/run-bru")
	if err != nil {
		t.Fatal(err)
	}
	wf := twoStepWorkflow("Auth/Login", "Me")

	var out bytes.Buffer
	result, err := Run(wf, col, Options{
		Vars: map[string]string{"BASE_URL": srv.URL},
		Out:  &out, In: strings.NewReader(""),
	})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	if !result.Success {
		t.Fatalf("run failed:\n%s", out.String())
	}
	if len(result.Steps) != 2 || result.Steps[1].Status != 200 {
		t.Fatalf("steps = %+v", result.Steps)
	}
}

func TestRunPropagatesCapturedAuthYAML(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	col, err := collection.Open("../../testdata/run-yaml")
	if err != nil {
		t.Fatal(err)
	}
	wf := twoStepWorkflow("Login", "Me")

	var out bytes.Buffer
	result, err := Run(wf, col, Options{
		Vars: map[string]string{"BASE_URL": srv.URL},
		Out:  &out, In: strings.NewReader(""),
	})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	if !result.Success {
		t.Fatalf("run failed:\n%s", out.String())
	}
	if len(result.Steps) != 2 || result.Steps[1].Status != 200 {
		t.Fatalf("steps = %+v", result.Steps)
	}
}

func TestRunPauseContinueAndAbort(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	col, err := collection.Open("../../testdata/run-bru")
	if err != nil {
		t.Fatal(err)
	}

	wf := &workflow.Workflow{
		Name: "pause-test",
		Steps: []workflow.Step{
			{Request: "Auth/Login", Name: "Login", Pause: "check something"},
		},
	}

	t.Run("continue", func(t *testing.T) {
		var out bytes.Buffer
		result, err := Run(wf, col, Options{
			Vars: map[string]string{"BASE_URL": srv.URL},
			Out:  &out, In: strings.NewReader("\n"),
		})
		if err != nil {
			t.Fatalf("Run: %v\n%s", err, out.String())
		}
		if result.Aborted {
			t.Fatal("should not have aborted")
		}
	})

	t.Run("abort", func(t *testing.T) {
		var out bytes.Buffer
		result, err := Run(wf, col, Options{
			Vars: map[string]string{"BASE_URL": srv.URL},
			Out:  &out, In: strings.NewReader("q\n"),
		})
		if err != nil {
			t.Fatalf("Run: %v\n%s", err, out.String())
		}
		if !result.Aborted {
			t.Fatal("should have aborted")
		}
	})
}
