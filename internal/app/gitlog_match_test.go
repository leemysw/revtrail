package app

import (
	"slices"
	"testing"

	"revtrail/internal/git"
)

// As IntelliJ's vcs.log.filter.text.highlight.matches, the log marks what
// its text filter matched, by its regular expression and case options.
func TestLogMatches(t *testing.T) {
	for _, tc := range []struct {
		s    string
		f    git.LogFilter
		want []int
	}{
		{"Add side B", git.LogFilter{Text: "side"}, []int{4, 5, 6, 7}},
		{"Add Side", git.LogFilter{Text: "side", CaseSensitive: true}, nil},
		{"fix a.b", git.LogFilter{Text: "a.b"}, []int{4, 5, 6}},
		{"v1 v22", git.LogFilter{Text: "v[0-9]+", Regexp: true}, []int{0, 1, 3, 4, 5}},
		{"修复 bug", git.LogFilter{Text: "修复"}, []int{0, 3}}, // where each character starts
		{"anything", git.LogFilter{Text: "("}, nil},
	} {
		if got := logMatches(tc.s, tc.f); !slices.Equal(got, tc.want) {
			t.Errorf("%q in %q: %v, want %v", tc.f.Text, tc.s, got, tc.want)
		}
	}
}
