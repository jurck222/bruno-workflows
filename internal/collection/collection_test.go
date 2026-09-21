package collection

import "testing"

func TestOpenAndLoadBru(t *testing.T) {
	c, err := Open("../../testdata/collection-bru")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if c.Format != FormatBru {
		t.Fatalf("Format = %v, want FormatBru", c.Format)
	}

	req, err := c.Load("Widgets/Item")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	checkWidgetItem(t, req)
}

func TestOpenAndLoadYAML(t *testing.T) {
	c, err := Open("../../testdata/collection-yaml")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if c.Format != FormatYAML {
		t.Fatalf("Format = %v, want FormatYAML", c.Format)
	}

	req, err := c.Load("widgets/Item")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	checkWidgetItem(t, req)
}

func checkWidgetItem(t *testing.T, req *Request) {
	t.Helper()
	var found bool
	for _, kv := range req.Vars {
		if kv.K == "widget_kind" && kv.V == "gadget" {
			found = true
		}
	}
	if !found {
		t.Errorf("folder variable widget_kind=gadget not visible in Vars: %+v", req.Vars)
	}
	if req.Auth == nil || req.Auth.Type != "bearer" || req.Auth.Token != "folder-token-123" {
		t.Errorf("Auth = %+v, want inherited bearer folder-token-123", req.Auth)
	}
}

func TestLoadDuplicateStemErrors(t *testing.T) {
	c, err := Open("../../testdata/collection-bru")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := c.Load("Widgets/Dup"); err == nil {
		t.Fatal("Load(Widgets/Dup) should error: both .bru and .yml exist")
	}

	c2, err := Open("../../testdata/collection-yaml")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := c2.Load("widgets/Dup"); err == nil {
		t.Fatal("Load(widgets/Dup) should error: both .bru and .yml exist")
	}
}
