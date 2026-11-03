package bash

import (
	"slices"
	"strings"
)

// wrapper is a program that runs the command in its arguments.
type wrapper struct {
	valued   string // Its short options that take a value.
	none     string // Its short options that make it run no command, as command -v does.
	operands int    // How many words come before the command, such as timeout's duration.
}

// wrappers are the wrappers, by name. Homebrew names GNU's tools with a g in front, such as
// gtimeout, and no wrapper's own name starts with a g. The NAME=value words that env and sudo
// take before the command are skipped after any wrapper, since no program's name holds a =.
var wrappers = map[string]wrapper{
	"builtin":    {},
	"caffeinate": {valued: "tw"},
	"command":    {none: "vV"},
	"env":        {valued: "CLPSUu"},
	"exec":       {valued: "a"},
	"nice":       {valued: "n"},
	"nohup":      {},
	"stdbuf":     {valued: "eio"},
	"sudo":       {valued: "CDgpRrTtUu"},
	"time":       {valued: "fo"},
	"timeout":    {valued: "ks", operands: 1},
	"xargs":      {valued: "EIJLPRSadns"},
}

// shells are the shells whose -c option runs the script in their first argument.
var shells = []string{"bash", "dash", "ksh", "sh", "zsh"}

// unwrap returns the words of the command that the words' wrappers run, or none if they run none.
func unwrap(words []string) []string {
	for len(words) > 0 {
		own, found := wrappers[strings.TrimPrefix(Name(words[0]), "g")]
		if !found {
			return words
		}

		letters, rest := options(words[1:], own.valued)
		if strings.ContainsAny(letters, own.none) {
			return nil
		}

		for len(rest) > 0 && strings.Contains(rest[0], "=") {
			rest = rest[1:]
		}

		words = rest[min(own.operands, len(rest)):]
	}

	return nil
}

// scriptOf returns the script that the words run, if they run one: that of a shell's -c option,
// or eval's words, joined by spaces, as eval joins them.
func scriptOf(words []string) (string, bool) {
	name := Name(words[0])
	if name == "eval" {
		return strings.Join(words[1:], " "), true
	}

	if !slices.Contains(shells, name) {
		return "", false
	}

	letters, args := options(words[1:], "oO")
	if !strings.Contains(letters, "c") || len(args) == 0 {
		return "", false
	}

	return args[0], true
}

// options reads the options that lead the words, as getopt does, and returns the letters of the
// short ones and the words after them. A letter in valued takes the rest of its word as its
// value, or else the next word. A long option, such as --name=value, holds its own value, and
// "--" and "-" hold none.
func options(words []string, valued string) (string, []string) {
	var letters strings.Builder

	for len(words) > 0 && strings.HasPrefix(words[0], "-") {
		word := words[0]
		words = words[1:]

		if strings.HasPrefix(word, "--") {
			continue
		}

		own, next := cluster(word[1:], valued)
		letters.WriteString(own)

		if next && len(words) > 0 {
			words = words[1:]
		}
	}

	return letters.String(), words
}

// cluster returns the letters of a word of short options, such as "xvf", up to the first that
// takes a value, and whether that one takes the next word, since its own word has none left.
func cluster(word, valued string) (string, bool) {
	end := strings.IndexAny(word, valued)
	if end < 0 {
		return word, false
	}

	return word[:end+1], end == len(word)-1
}
