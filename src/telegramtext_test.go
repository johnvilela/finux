package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	for input, want := range map[string][]string{
		"/ac":                                   {"/ac"},
		"  /tx   add  --value 45 ":              {"/tx", "add", "--value", "45"},
		`/tx add --title "Weekly market" -v 1`:  {"/tx", "add", "--title", "Weekly market", "-v", "1"},
		`/tx add --title 'It is "ok"'`:          {"/tx", "add", "--title", `It is "ok"`},
		`/tx add --title “Bus ticket” —value 5`: {"/tx", "add", "--title", "Bus ticket", "--value", "5"},
		`/tx add --title a\ b --description ""`: {"/tx", "add", "--title", "a b", "--description", ""},
		"/tx add --title Pão — de queijo –x":    {"/tx", "add", "--title", "Pão", "—", "de", "queijo", "--x"},
		"/tx\nadd\t--value=45":                  {"/tx", "add", "--value=45"},
		`/tx add --title Joe's --tags it’s`:     {"/tx", "add", "--title", "Joe's", "--tags", "it’s"},
		`/tx add --title="a b"`:                 {"/tx", "add", "--title=a b"},
		`/tx add --title "no end`:               nil,
		"":                                      {},
	} {
		got, err := splitArgs(input)
		if want == nil {
			if err == nil {
				t.Errorf("splitArgs(%q) = %q, want an error", input, got)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, want) && (len(got) != 0 || len(want) != 0) {
			t.Errorf("splitArgs(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestChunks(t *testing.T) {
	if got := chunks("", 10); len(got) != 0 {
		t.Errorf("chunks of no text = %q", got)
	}
	if got := chunks("one\ntwo\n", 10); !reflect.DeepEqual(got, []string{"one\ntwo"}) {
		t.Errorf("chunks of a short text = %q", got)
	}
	// A cut is at the end of a line when a line fits.
	if got := chunks("aaaa\nbbbb\ncccc\n", 10); !reflect.DeepEqual(got, []string{"aaaa\nbbbb", "cccc"}) {
		t.Errorf("chunks of lines = %q", got)
	}
	// A line that is too long is cut between characters, not in one.
	long := strings.Repeat("é", 25)
	got := chunks(long, 10)
	if len(got) != 3 || strings.Join(got, "") != long || got[0] != strings.Repeat("é", 10) {
		t.Errorf("chunks of a long line = %q", got)
	}
}
