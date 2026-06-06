package safety

import (
	"strings"
	"unicode"
)

// State represents the current scanning state
type State int

const (
	StateNormal State = iota
	StateInString
	StateInLineComment
	StateInBlockComment
)

// Keywords that indicate write operations
var writeKeywords = map[string]bool{
	"INSERT":  true,
	"UPDATE":  true,
	"DELETE":  true,
	"DROP":    true,
	"ALTER":   true,
	"CREATE":  true,
	"TRUNCATE": true,
	"REPLACE": true,
	"GRANT":   true,
	"REVOKE":  true,
	"LOCK":    true,
}

// ScanKeywords scans SQL and returns detected write keywords
// Single-pass scanner that tracks state to avoid false positives
func ScanKeywords(sql string) []string {
	var keywords []string
	state := StateNormal
	var currentWord strings.Builder

	i := 0
	n := len(sql)

	for i < n {
		ch := sql[i]

		switch state {
		case StateNormal:
			// Check for state transitions
			if ch == '\'' {
				state = StateInString
				i++
				flushWord(&currentWord, &keywords)
			} else if i+1 < n && ch == '-' && sql[i+1] == '-' {
				state = StateInLineComment
				i += 2
				flushWord(&currentWord, &keywords)
			} else if i+1 < n && ch == '/' && sql[i+1] == '*' {
				state = StateInBlockComment
				i += 2
				flushWord(&currentWord, &keywords)
			} else if isWordChar(rune(ch)) {
				currentWord.WriteRune(unicode.ToUpper(rune(ch)))
				i++
			} else {
				// Word boundary
				flushWord(&currentWord, &keywords)
				i++
			}

		case StateInString:
			if ch == '\'' {
				// Check for escaped quote ''
				if i+1 < n && sql[i+1] == '\'' {
					// Escaped quote, skip both
					i += 2
				} else {
					// End of string
					state = StateNormal
					i++
				}
			} else {
				i++
			}

		case StateInLineComment:
			if ch == '\n' {
				state = StateNormal
			}
			i++

		case StateInBlockComment:
			if i+1 < n && ch == '*' && sql[i+1] == '/' {
				state = StateNormal
				i += 2
			} else {
				i++
			}
		}
	}

	// Flush any remaining word
	flushWord(&currentWord, &keywords)

	return keywords
}

// isWordChar returns true if character can be part of a keyword
func isWordChar(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

// flushWord checks if current word is a keyword and adds to list
func flushWord(word *strings.Builder, keywords *[]string) {
	if word.Len() == 0 {
		return
	}

	w := word.String()
	if writeKeywords[w] {
		// Check if keyword already in list (avoid duplicates)
		found := false
		for _, kw := range *keywords {
			if kw == w {
				found = true
				break
			}
		}
		if !found {
			*keywords = append(*keywords, w)
		}
	}

	word.Reset()
}