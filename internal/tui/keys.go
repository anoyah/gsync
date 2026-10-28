package tui

import (
	"bufio"
	"context"
	"io"
)

func readKeys(ctx context.Context, in io.Reader, keys chan<- string) {
	defer close(keys)
	r := bufio.NewReader(in)
	send := func(key string) bool {
		select {
		case keys <- key:
			return true
		case <-ctx.Done():
			return false
		}
	}
	paste := false
	for {
		b, err := r.ReadByte()
		if err != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		if b == 27 {
			next, err := r.ReadByte()
			if err != nil {
				return
			}
			if next != '[' {
				continue
			}
			sequence := ""
			for len(sequence) < 32 {
				c, err := r.ReadByte()
				if err != nil {
					return
				}
				sequence += string(c)
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
			if sequence == "200~" {
				paste = true
				continue
			}
			if sequence == "201~" {
				paste = false
				continue
			}
			if paste {
				continue
			}
			if sequence == "A" {
				if !send("up") {
					return
				}
			}
			if sequence == "B" {
				if !send("down") {
					return
				}
			}
			continue
		}
		if paste {
			continue
		}
		key := ""
		switch b {
		case 'q', 3, 4:
			key = "quit"
		case 'w':
			key = "watch"
		case 's':
			key = "start"
		case 'x':
			key = "stop"
		case 'r':
			key = "refresh"
		case 'k':
			key = "up"
		case 'j':
			key = "down"
		case 'G':
			key = "latest"
		case '\t':
			key = "tab"
		}
		if key != "" && !send(key) {
			return
		}
	}
}
