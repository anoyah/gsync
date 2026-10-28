package main

import (
	"bytes"
	"context"
	"testing"
)

func TestFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0}, {[]string{"-h"}, 0}, {[]string{"--version"}, 0},
		{[]string{"--unknown"}, 2}, {[]string{"--timeout=0"}, 2}, {[]string{"--timeout=bad"}, 2}, {[]string{"extra"}, 2},
		{[]string{"--interval=0"}, 2}, {[]string{"--interval=-1s"}, 2}, {[]string{"--interval=bad"}, 2},
		{[]string{"start", "--help"}, 0}, {[]string{"stop", "--help"}, 0}, {[]string{"status", "--help"}, 0}, {[]string{"logs", "--help"}, 0},
		{[]string{"start", "--once"}, 2}, {[]string{"start", "--dry-run"}, 2}, {[]string{"start", "--version"}, 2},
		{[]string{"status", "--interval=5s"}, 2}, {[]string{"logs", "--lines=0"}, 2}, {[]string{"logs", "--lines=10001"}, 2},
		{[]string{"stop", "--verbose"}, 2},
		{[]string{"tui", "--help"}, 0}, {[]string{"tui", "--once"}, 2}, {[]string{"tui", "--dry-run"}, 2},
		{[]string{"tui"}, 1},
		{[]string{"watch", "--help"}, 0},
	} {
		var out bytes.Buffer
		if got := run(context.Background(), tc.args, &out, &out); got != tc.code {
			t.Fatalf("%v: %d: %s", tc.args, got, out.String())
		}
	}
}
