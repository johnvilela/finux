package main

import (
	"errors"
	"strings"
	"unicode"
)

// errOpenQuote means a quote in a message has no end.
var errOpenQuote = errors.New("a quote has no end")

// The quotes that a phone keyboard writes, with the plain ones.
const (
	doubleQuotes = "\"“”„"
	singleQuotes = "'‘’"
)

// splitArgs splits the text of a message into arguments, as a shell
// does: spaces are between arguments, and quotes keep the spaces of an
// argument. A quote starts an argument or comes after "=", so the
// apostrophe of a word is not a quote. A backslash keeps the character
// after it. A phone changes "--" to a long dash and quotes to curly
// quotes: splitArgs reads them as the plain ones.
func splitArgs(text string) ([]string, error) {
	args := []string{}
	var current []rune
	started, quoted := false, false
	closing := ""
	end := func() {
		if !started {
			return
		}
		word := string(current)
		if !quoted {
			for _, dash := range []string{"—", "–"} {
				if rest, ok := strings.CutPrefix(word, dash); ok && rest != "" && !strings.ContainsAny(rest[:1], "—– ") {
					word = "--" + rest
				}
			}
		}
		args = append(args, word)
		current, started, quoted = nil, false, false
	}
	// opens reports whether a quote at this place starts a quoted text.
	opens := func() bool {
		return !started || len(current) > 0 && current[len(current)-1] == '='
	}
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		char := runes[i]
		switch {
		case closing != "" && strings.ContainsRune(closing, char):
			closing = ""
		case char == '\\' && i+1 < len(runes) && closing != singleQuotes:
			i++
			current, started = append(current, runes[i]), true
		case closing != "":
			current = append(current, char)
		case unicode.IsSpace(char):
			end()
		case opens() && strings.ContainsRune(doubleQuotes, char):
			closing, quoted, started = doubleQuotes, !started, true
		case opens() && strings.ContainsRune(singleQuotes, char):
			closing, quoted, started = singleQuotes, !started, true
		default:
			current, started = append(current, char), true
		}
	}
	if closing != "" {
		return nil, errOpenQuote
	}
	end()
	return args, nil
}

// chunks cuts a text into parts of limit characters or fewer, for
// messages. It cuts at the end of a line when that is possible.
func chunks(text string, limit int) []string {
	var parts []string
	var part []rune
	flush := func() {
		if len(part) > 0 {
			parts = append(parts, string(part))
			part = nil
		}
	}
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		runes := []rune(line)
		if len(part) > 0 && len(part)+1+len(runes) > limit {
			flush()
		}
		for len(runes) > limit {
			parts = append(parts, string(runes[:limit]))
			runes = runes[limit:]
		}
		if len(part) > 0 {
			part = append(part, '\n')
		}
		part = append(part, runes...)
	}
	flush()
	return parts
}
