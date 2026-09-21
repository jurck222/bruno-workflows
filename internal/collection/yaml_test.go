package collection

import (
	"os"
	"strings"
	"testing"
)

func TestParseYAMLArrayHeadersAndParams(t *testing.T) {
	req, err := parseYAMLRequest("../../testdata/yml-collection/fixture-headers/Units.yml")
	if err != nil {
		t.Fatalf("parseYAMLRequest: %v", err)
	}
	if req.Method != "GET" {
		t.Errorf("Method = %q, want GET", req.Method)
	}
	if len(req.Headers) != 2 || req.Headers[0].K != "API-KEY" {
		t.Fatalf("Headers = %+v", req.Headers)
	}
	if len(req.Query) != 1 || req.Query[0].K != "duration" {
		t.Fatalf("Query = %+v", req.Query)
	}
	if req.Auth == nil || req.Auth.Type != "inherit" {
		t.Fatalf("Auth = %+v, want inherit", req.Auth)
	}
}

func TestParseYAMLMultipartFileSequence(t *testing.T) {
	req, err := parseYAMLRequest("../../testdata/yml-collection/fixture-multipart/backend/Import users.yml")
	if err != nil {
		t.Fatalf("parseYAMLRequest: %v", err)
	}
	if req.Body.Kind != "multipart-form" {
		t.Fatalf("Body.Kind = %q, want multipart-form", req.Body.Kind)
	}
	if len(req.Body.Parts) != 1 || !req.Body.Parts[0].IsFile {
		t.Fatalf("Parts = %+v, want one file part", req.Body.Parts)
	}
	if req.Body.Parts[0].Value != "sample-data/users/sample-import.csv" {
		t.Errorf("file value = %q", req.Body.Parts[0].Value)
	}
	if req.Auth == nil || req.Auth.Type != "bearer" {
		t.Fatalf("Auth = %+v, want bearer", req.Auth)
	}
}

func TestParseYAMLRuntimeScriptsIgnored(t *testing.T) {
	req, err := parseYAMLRequest("../../testdata/yml-collection/fixture-scripts/Login.yml")
	if err != nil {
		t.Fatalf("parseYAMLRequest with runtime.scripts should not error: %v", err)
	}
	if req.Body.Kind != "json" || req.Body.Raw == "" {
		t.Fatalf("Body = %+v", req.Body)
	}
}

func TestParseYAMLUnsupportedAuthAndType(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "oauth2 auth",
			yaml: "info:\n  name: X\n  type: http\nhttp:\n  method: GET\n  url: x\n  auth: {type: oauth2}\n",
			want: `auth type "oauth2" is not supported`,
		},
		{
			name: "grpc type",
			yaml: "info:\n  name: X\n  type: grpc\nhttp:\n  method: GET\n  url: x\n",
			want: `unsupported request type "grpc"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := dir + "/req.yml"
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := parseYAMLRequest(path)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tc.want)
			}
		})
	}
}
