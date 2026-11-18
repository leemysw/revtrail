package syntax

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func goKinds(t *testing.T, line string) map[string]editor.TokenKind {
	t.Helper()
	tokens, _ := For("a.go").Tokenizer.Tokenize(line, 0, nil)
	kinds := map[string]editor.TokenKind{}
	start := 0
	for _, tok := range tokens {
		kinds[line[start:tok.End]] = tok.Kind
		start = tok.End
	}
	return kinds
}

func TestGoFunctionNames(t *testing.T) {
	for _, c := range []struct {
		line, word string
		want       editor.TokenKind
	}{
		{"func Open(path string) error {", "Open", editor.TokenFunction},
		{"func (r *Repo) Log[T any](n int) {", "Log", editor.TokenFunction},
		{"func Map[T any](xs []T) {", "Map", editor.TokenFunction},
		{"	fmt.Println(len(xs))", "Println", editor.TokenMeta},
		{"	fmt.Println(len(xs))", "len", editor.TokenType},
		{"	f := func(a int) error { return run(a) }", "run", editor.TokenMeta},
		{"	return nil", "nil", editor.TokenConstant},
	} {
		kinds := goKinds(t, c.line)
		got, ok := kinds[c.word]
		if !ok {
			for w, k := range kinds {
				if len(w) > len(c.word) && w[len(w)-len(c.word):] == c.word {
					got, ok = k, true
				}
			}
		}
		if !ok || got != c.want {
			t.Errorf("%q: %q is %v, want %v (%v)", c.line, c.word, got, c.want, kinds)
		}
	}
}
