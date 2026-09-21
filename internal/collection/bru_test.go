package collection

import (
	"encoding/json"
	"testing"
)

func TestParseBruLogin(t *testing.T) {
	req, err := parseBru("../../testdata/bru-collection/Auth/Login.bru")
	if err != nil {
		t.Fatalf("parseBru: %v", err)
	}
	if req.Name != "Login" {
		t.Errorf("Name = %q, want Login", req.Name)
	}
	if req.Method != "POST" {
		t.Errorf("Method = %q, want POST", req.Method)
	}
	if req.Body.Kind != "json" {
		t.Fatalf("Body.Kind = %q, want json", req.Body.Kind)
	}
	if !json.Valid([]byte(req.Body.Raw)) {
		t.Fatalf("Body.Raw is not valid JSON after comment stripping: %q", req.Body.Raw)
	}
	if req.Auth == nil || req.Auth.Type != "inherit" {
		t.Errorf("Auth = %+v, want inherit", req.Auth)
	}
}

func TestParseBruUploadCaseFile(t *testing.T) {
	req, err := parseBru("../../testdata/bru-collection/Case/Upload case file.bru")
	if err != nil {
		t.Fatalf("parseBru: %v", err)
	}
	if req.Body.Kind != "multipart-form" {
		t.Fatalf("Body.Kind = %q, want multipart-form", req.Body.Kind)
	}
	if len(req.Body.Parts) != 1 || !req.Body.Parts[0].IsFile {
		t.Fatalf("Parts = %+v, want one file part", req.Body.Parts)
	}
	if req.Body.Parts[0].Value != "/path/to/your/file.pdf" {
		t.Errorf("file path = %q", req.Body.Parts[0].Value)
	}
}

func TestParseBruGetSignedURL(t *testing.T) {
	req, err := parseBru("../../testdata/bru-collection/Case/Get signed url.bru")
	if err != nil {
		t.Fatalf("parseBru: %v", err)
	}
	if req.Method != "POST" || req.Body.Kind != "json" {
		t.Fatalf("got method=%s body.kind=%s", req.Method, req.Body.Kind)
	}
}

func TestParseBruHonorsBodySelectorOverStaleBlock(t *testing.T) {
	req, err := parseBru("../../testdata/bru-collection/Stale body select.bru")
	if err != nil {
		t.Fatalf("parseBru: %v", err)
	}
	if req.Body.Kind != "" {
		t.Fatalf("Body = %+v, want no body (selector is \"none\", body:json block is stale)", req.Body)
	}
}

func TestParseBruSetSessionStatusNoHeaders(t *testing.T) {
	req, err := parseBru("../../testdata/bru-collection/Test/Set session status.bru")
	if err != nil {
		t.Fatalf("parseBru: %v", err)
	}
	if req.Method != "PATCH" {
		t.Errorf("Method = %q, want PATCH", req.Method)
	}
	if len(req.Headers) != 0 {
		t.Errorf("Headers = %+v, want none", req.Headers)
	}
}
