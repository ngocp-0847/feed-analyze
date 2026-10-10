package main

import (
	"slices"
	"testing"
)

func TestParseFlagsInterleaved(t *testing.T) {
	fs := newFlags("t")
	days := fs.Int("days", 1, "")
	js := fs.Bool("json", false, "")
	pos, err := parseFlags(fs, []string{"lãi", "--days", "7", "suất", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if *days != 7 || !*js || !slices.Equal(pos, []string{"lãi", "suất"}) {
		t.Fatalf("days=%d json=%v pos=%v", *days, *js, pos)
	}
}
