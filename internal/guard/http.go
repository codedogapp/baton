package guard

import (
	"maps"
	"slices"
	"strings"

	"github.com/codedogapp/baton/internal/bash"
)

// part is what an HTTP client's option sets in its request.
type part int

// The parts of a request that the guard reads.
const (
	// method is the request's method, such as POST.
	method part = iota + 1
	// body is data to send, which makes a request with no method a POST or a PUT.
	body
	// setting is a line of wget's settings, such as method=POST.
	setting
	// next starts another request with options of its own, as curl's --next does.
	next
	// unseen is an option that only the run decides, which could be any of them.
	unseen
)

// client is the options of an HTTP client that the guard reads.
type client struct {
	short map[byte]part
	long  map[string]part
	// valued are the client's other short options that take a value, which hides the rest of
	// their word.
	valued string
}

// clients are the HTTP clients that the guard reads, by program.
var clients = map[string]client{
	"curl": {
		short: map[byte]part{':': next, 'F': body, 'T': body, 'X': method, 'd': body},
		long: map[string]part{
			"data": body, "data-ascii": body, "data-binary": body, "data-raw": body,
			"data-urlencode": body, "form": body, "form-string": body, "json": body, "next": next,
			"request": method, "upload-file": body,
		},
		valued: "AbcCDeEhHKmoPQrtuUwxyYz",
	},
	"wget": {
		short: map[byte]part{'e': setting},
		long: map[string]part{
			"body-data": body, "body-file": body, "execute": setting, "method": method,
			"post-data": body, "post-file": body,
		},
		valued: "aABDiIlnoOPQRtTUwX",
	},
}

// ghAPI is the options of gh api.
var ghAPI = client{
	short:  map[byte]part{'F': body, 'X': method, 'f': body},
	long:   map[string]part{"field": body, "input": body, "method": method, "raw-field": body},
	valued: "Hpqt",
}

// request is what an HTTP client's options ask of its requests.
type request struct {
	methods []string
	body    bool
	// wrote is whether the requests write, whatever the options after say: one before the last
	// --next does, or an option that only the run decides could make them.
	wrote bool
}

// writes reports whether an HTTP client's arguments make it send data: by a method other than GET
// or HEAD, or, with no method, by any data to send. Data that curl -G puts in the URL counts too,
// as does an option that only the run decides.
func writes(options client, args []string) bool {
	var req request

	for len(args) > 0 {
		args = req.option(options, args)
	}

	return req.writes()
}

// option reads the option in the first of the words, if it is one, and returns the words that
// follow it and any value that it takes.
func (r *request) option(options client, args []string) []string {
	word, rest := args[0], args[1:]

	if strings.HasPrefix(word, "--") {
		name, value, inline := strings.Cut(word[2:], "=")

		opt := options.lookup(name)
		if !inline && opt.valued() {
			return r.take(opt, rest)
		}

		r.set(opt, value)

		return rest
	}

	if !strings.HasPrefix(word, "-") {
		return rest
	}

	for letters := word[1:]; letters != ""; letters = letters[1:] {
		opt := options.letter(letters[0])

		switch {
		case !opt.valued() && !strings.Contains(options.valued, letters[:1]):
			r.set(opt, "")
		case len(letters) > 1:
			r.set(opt, letters[1:])

			return rest
		default:
			return r.take(opt, rest)
		}
	}

	return rest
}

// letter returns the part of a short option's letter.
func (c client) letter(b byte) part {
	if b == bash.Dynamic[0] {
		return unseen
	}

	return c.short[b]
}

// lookup returns the part of a long option, by its name or by the start of a name, as wget
// allows: the first name in order that starts so, which is the name itself if there is one. curl's
// --expand- names, which fill in variables, mean the options without the prefix.
func (c client) lookup(name string) part {
	name = strings.TrimPrefix(name, "expand-")

	switch {
	case strings.Contains(name, bash.Dynamic):
		return unseen
	case name == "":
		return 0
	}

	for _, long := range slices.Sorted(maps.Keys(c.long)) {
		if strings.HasPrefix(long, name) {
			return c.long[long]
		}
	}

	return 0
}

// take sets the part to the first of the words, or to an empty value if there is none, and returns
// the words after it.
func (r *request) take(p part, args []string) []string {
	if len(args) == 0 {
		args = []string{""}
	}

	r.set(p, args[0])

	return args[1:]
}

// set records an option's part, with its value if it takes one.
func (r *request) set(p part, value string) {
	switch p {
	case method:
		r.methods = append(r.methods, strings.ToUpper(value))
	case body:
		r.body = true
	case setting:
		r.set(config(value))
	case next:
		*r = request{wrote: r.writes()}
	case unseen:
		r.wrote = true
	}
}

// writes reports whether the requests send data: by a method other than GET or HEAD, or, with no
// method, by any data to send.
func (r *request) writes() bool {
	return r.wrote || slices.ContainsFunc(r.methods, writing) || (r.methods == nil && r.body)
}

// valued reports whether an option of the part takes a value.
func (p part) valued() bool {
	return p == method || p == body || p == setting
}

// writing reports whether a request's method sends data, as any but GET and HEAD may.
func writing(method string) bool {
	return method != "GET" && method != "HEAD"
}

// config returns the part that a line of wget's settings sets, such as method=POST, and its value.
// wget drops the case, - and _ of a setting's name. A name that only the run decides could be any.
func config(line string) (part, string) {
	name, value, _ := strings.Cut(line, "=")
	name = strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(name)))

	switch name {
	case "method":
		return method, strings.TrimSpace(value)
	case "bodydata", "bodyfile", "postdata", "postfile":
		return body, ""
	}

	if strings.Contains(name, bash.Dynamic) {
		return unseen, ""
	}

	return 0, ""
}
