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
	} {
		var out bytes.Buffer
		if got := run(context.Background(), tc.args, &out, &out); got != tc.code {
			t.Fatalf("%v: %d: %s", tc.args, got, out.String())
		}
	}
}
