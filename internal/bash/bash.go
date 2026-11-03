// Package bash reads a bash script, such as the command of a copilot's bash tool, and finds the
// commands that it runs (decision 50).
package bash

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Dynamic stands in a word's value for each of its parts that bash works out only as it runs the
// command, such as a variable or a command's output: a NUL byte, which no argument can hold.
const Dynamic = "\x00"

// Calls returns the commands that the script runs, each as its words, as bash passes them: those
// in command substitutions, here-documents and functions too; for a wrapper such as env, nohup or
// xargs, the command that it runs; and for sh -c and eval, the commands of their script. It
// returns an error if the script, or one that it runs, doesn't parse.
func Calls(script string) ([][]string, error) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))

	file, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		return nil, err
	}

	var (
		calls [][]string
		errs  []error
	)

	syntax.Walk(file, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok {
			found, err := runs(values(call.Args))
			calls, errs = append(calls, found...), append(errs, err)
		}

		return true
	})

	return calls, errors.Join(errs...)
}

// Name returns the name of the program that a command's first word runs: the word's last element,
// in lower case, since a Mac's file system ignores case.
func Name(word string) string {
	return strings.ToLower(filepath.Base(word))
}

// runs returns the commands that a command of these words runs: the command itself, the one that
// its wrappers run, or those of the script that it runs.
func runs(words []string) ([][]string, error) {
	words = unwrap(words)
	if len(words) == 0 {
		return nil, nil
	}

	script, found := scriptOf(words)
	if !found {
		return [][]string{words}, nil
	}

	// The parser drops NUL bytes, so each dynamic part of the script becomes a variable, whose
	// value is dynamic again.
	calls, err := Calls(strings.ReplaceAll(script, Dynamic, "${dynamic}"))
	if err != nil {
		return nil, fmt.Errorf("%s's script: %w", Name(words[0]), err)
	}

	return calls, nil
}

// values returns the values of the words, as bash passes them to a command.
func values(words []*syntax.Word) []string {
	texts := make([]string, len(words))
	for i, word := range words {
		texts[i] = value(word.Parts, false)
	}

	return texts
}

// value returns the value of a word's parts, inside double quotes or not, with Dynamic for each
// part that bash works out as it runs, and for $'…', whose escapes baton doesn't read.
func value(parts []syntax.WordPart, quoted bool) string {
	var text strings.Builder

	for _, part := range parts {
		switch part := part.(type) {
		case *syntax.Lit:
			text.WriteString(unescape(part.Value, quoted))
		case *syntax.SglQuoted:
			if part.Dollar {
				text.WriteString(Dynamic)
			} else {
				text.WriteString(part.Value)
			}
		case *syntax.DblQuoted:
			text.WriteString(value(part.Parts, true))
		default:
			text.WriteString(Dynamic)
		}
	}

	return text.String()
}

// unescape returns a literal's text without the backslashes that escape a character. The parser
// has dropped those that join two lines.
func unescape(text string, quoted bool) string {
	var plain strings.Builder

	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && escapes(text[i+1], quoted) {
			i++
		}

		plain.WriteByte(text[i])
	}

	return plain.String()
}

// escapes returns whether a backslash escapes the character: any character outside double quotes,
// and only $, `, " and \ inside them.
func escapes(char byte, quoted bool) bool {
	return !quoted || strings.IndexByte("$`\"\\", char) >= 0
}
