package guard_test

import (
	"fmt"
	"testing"
)

func TestCommandsFindsCurlsWrites(t *testing.T) {
	wantCommands(t, map[string][]string{
		"curl -d x https://h":                         {"outward: curl"},
		"curl --data=x https://h":                     {"outward: curl"},
		"curl --data-ascii x https://h":               {"outward: curl"},
		"curl --data-binary @f https://h":             {"outward: curl"},
		"curl --data-raw x https://h":                 {"outward: curl"},
		"curl --data-urlencode q=x https://h":         {"outward: curl"},
		"curl -F f=@x https://h":                      {"outward: curl"},
		"curl --form f=@x https://h":                  {"outward: curl"},
		"curl --form-string f=x https://h":            {"outward: curl"},
		"curl --json '{}' https://h":                  {"outward: curl"},
		"curl -T x sftp://h/":                         {"outward: curl"},
		"curl --upload-file x https://h":              {"outward: curl"},
		"curl --expand-data '{{x}}' https://h":        {"outward: curl"},
		"curl -X DELETE https://h/x":                  {"outward: curl"},
		"curl --request patch https://h":              {"outward: curl"},
		"curl --request=PUT https://h":                {"outward: curl"},
		"curl -sSXPOST https://h":                     {"outward: curl"},
		"curl -X GET -X POST https://h":               {"outward: curl"},
		"curl -G -d q=x https://h":                    {"outward: curl"},
		"curl -d x https://a --next https://b":        {"outward: curl"},
		"curl -X GET https://a --next -d x https://b": {"outward: curl"},
		"curl -X GET https://a -: -d x https://b":     {"outward: curl"},
		"curl https://h -X":                           {"outward: curl"},
		`curl -X "$method" https://h`:                 {"outward: curl"},
		`curl -$letter POST https://h`:                {"outward: curl"},
		`curl --"$name"=x https://h`:                  {"outward: curl"},
		"CURL -d x https://h":                         {"outward: curl"},
	})
}

func TestCommandsLetsCurlsReadsBe(t *testing.T) {
	wantCommands(t, map[string][]string{
		"curl":                                         nil,
		"curl -fsSL -o out https://h/x":                nil,
		"curl -I https://h":                            nil,
		"curl -X HEAD https://h":                       nil,
		"curl -X get https://h":                        nil,
		"curl --request=GET https://h":                 nil,
		"curl -X GET -d x https://h":                   nil,
		"curl -X GET -d -XPOST https://h":              nil,
		"curl -H'X-Debug: 1' https://h":                nil,
		"curl -X GET https://a --next https://b":       nil,
		"curl --no-progress-meter https://h":           nil,
		"curl --output-dir ./downloads -O https://h/f": nil,
		"curl -- https://h":                            nil,
		`curl "$url"`:                                  nil,
		`curl -o"$out" --output="$out" https://h`:      nil,
	})
}

func TestCommandsFindsWgetsWrites(t *testing.T) {
	wantCommands(t, map[string][]string{
		"wget --post-data=x https://h":               {"outward: wget"},
		"wget -qO- --post-data=x https://h":          {"outward: wget"},
		"wget --post-file f https://h":               {"outward: wget"},
		"wget --body-data x https://h":               {"outward: wget"},
		"wget --body-file=f https://h":               {"outward: wget"},
		"wget --method=PUT --body-file=f https://h":  {"outward: wget"},
		"wget --body-data x --method POST https://h": {"outward: wget"},
		"wget --method DELETE https://h/x":           {"outward: wget"},
		"wget --meth=POST https://h":                 {"outward: wget"},
		"wget --post-d=x https://h":                  {"outward: wget"},
		"wget -e method=POST https://h":              {"outward: wget"},
		"wget -qemethod=put https://h":               {"outward: wget"},
		"wget -e 'Post_Data = x' https://h":          {"outward: wget"},
		"wget -e body_data=x https://h":              {"outward: wget"},
		"wget -e BodyFile=f https://h":               {"outward: wget"},
		"wget --execute=post-file=f https://h":       {"outward: wget"},
		`wget -e "$setting" https://h`:               {"outward: wget"},
		`wget -e "method=$method" https://h`:         {"outward: wget"},
	})
}

func TestCommandsLetsWgetsReadsBe(t *testing.T) {
	wantCommands(t, map[string][]string{
		"wget https://h/x":                 nil,
		"wget -qO- https://h/x":            nil,
		"wget -nv -O - https://h/x":        nil,
		"wget --spider https://h":          nil,
		"wget --method=HEAD https://h":     nil,
		"wget -e 'method = get' https://h": nil,
		"wget -e robots=off -r https://h":  nil,
	})
}

func TestCommandsFindsGhAPIsWrites(t *testing.T) {
	wantCommands(t, map[string][]string{
		"gh api -X POST repos/o/r/dispatches":         {"outward: gh api"},
		"gh api repos/o/r/issues -f title=x":          {"outward: gh api"},
		"gh api repos/o/r/issues -F n=1":              {"outward: gh api"},
		"gh api repos/o/r/issues --raw-field title=x": {"outward: gh api"},
		"gh api repos/o/r/issues --field n=1":         {"outward: gh api"},
		"gh api repos/o/r/issues --input body.json":   {"outward: gh api"},
		"gh api --method=PATCH repos/o/r":             {"outward: gh api"},
		"gh api -XDELETE repos/o/r/git/refs/heads/x":  {"outward: gh api"},
		"gh api graphql -X GET -f query=x":            {"outward: gh api"},
		`gh api -X "$method" repos/o/r`:               {"outward: gh api"},
		// gh reads -X=GET as GET, but the guard fails closed on it.
		"gh api -X=GET repos/o/r": {"outward: gh api"},
	})
}

func TestCommandsLetsGhAPIsReadsBe(t *testing.T) {
	wantCommands(t, map[string][]string{
		"gh api repos/o/r":                                    nil,
		"gh api -i repos/o/r":                                 nil,
		"gh api -X GET repos/o/r/pulls":                       nil,
		"gh api repos/o/r/pulls -X GET -f state=all":          nil,
		"gh api --method head repos/o/r":                      nil,
		"gh api --paginate repos/o/r/issues --jq '.[].title'": nil,
	})
}

// TestCommandsSkipsTheValuesOfShortOptions checks that each short option that takes a value, as the
// client's help shows, takes the next word, which would write if it were an option.
func TestCommandsSkipsTheValuesOfShortOptions(t *testing.T) {
	cases := map[string][]string{}

	for _, client := range []struct{ program, letters, option string }{
		{"curl", "AbcCDeEhHKmoPQrtuUwxyYz", "-XPOST"},
		{"wget", "aABDiIlnoOPQRtTUwX", "--method=POST"},
		{"gh api", "Hpqt", "-XPOST"},
	} {
		cases[client.program+" "+client.option+" u"] = []string{"outward: " + client.program}

		for _, letter := range client.letters {
			cases[fmt.Sprintf("%s -%c %s u", client.program, letter, client.option)] = nil
		}
	}

	wantCommands(t, cases)
}
