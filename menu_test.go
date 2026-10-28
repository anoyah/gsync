package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNoArgumentsShowsMenu(t *testing.T) {
	var out, diagnostic bytes.Buffer
	if code := run(context.Background(), nil, &out, &diagnostic); code != 0 {
		t.Fatalf("%d: %s", code, diagnostic.String())
	}
	for _, operation := range []string{"gsync tui", "gsync watch", "gsync --once", "gsync start", "gsync stop", "gsync status", "gsync logs", "gsync --help"} {
		if !strings.Contains(out.String(), operation) {
			t.Fatalf("missing %s", operation)
		}
	}
	if strings.Contains(out.String(), "请选择操作") {
		t.Fatal("operation list should not prompt")
	}
}
