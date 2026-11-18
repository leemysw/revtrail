// INPUT: mygo/ui/editor 自带的 Go 语言与其分词结果。
// OUTPUT: goLanguage：在自带分词上区分 Go 的内建函数、函数声明与调用，好按 GoLand 的配色上色。
// POS: Go 的语法高亮；颜色在 internal/theme。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package syntax

import (
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

// goLanguage is the editor's Go, whose function names GoLand colors three
// ways: a builtin as the builtin types (TokenType), a declaration as one
// (TokenFunction), and a call as TokenMeta, which Go has no other use for.
var goLanguage = func() *editor.Language {
	base := editor.LanguageFor("x.go")
	if base == nil {
		return nil
	}
	lang := *base
	inner := base.Tokenizer
	lang.Tokenizer = editor.TokenizerFunc(func(line string, state uint64, tokens []editor.Token) ([]editor.Token, uint64) {
		from := len(tokens)
		tokens, state = inner.Tokenize(line, state, tokens)
		refineGo(line, tokens, from)
		return tokens, state
	})
	return &lang
}()

var goBuiltins = map[string]bool{}

func init() {
	for _, name := range strings.Fields("append cap clear close complex copy delete imag len make max min new panic print println real recover") {
		goBuiltins[name] = true
	}
}

// refineGo recolors the function names of a line's tokens from the
// from-th: the name after func, or after a method's receiver, is a
// declaration; a builtin's is a builtin; the rest are calls.
func refineGo(line string, tokens []editor.Token, from int) {
	start := 0
	if from > 0 {
		start = tokens[from-1].End
	}
	const (
		none = iota
		afterFunc
		inReceiver
		afterReceiver
	)
	decl, depth, first := none, 0, true
	for i := from; i < len(tokens); i++ {
		word := strings.TrimSpace(line[start:tokens[i].End])
		start = tokens[i].End
		if word == "" {
			continue
		}
		kind := tokens[i].Kind
		lineStart := first
		first = false
		switch {
		case kind == editor.TokenKeyword && word == "func":
			decl = none // a function literal: what follows is its signature
			if lineStart {
				decl = afterFunc
			}
			continue
		case decl == afterFunc && strings.HasPrefix(word, "("):
			decl, depth = inReceiver, 0
			fallthrough
		case decl == inReceiver:
			depth += strings.Count(word, "(") - strings.Count(word, ")")
			if depth <= 0 {
				decl = afterReceiver
			}
			continue
		case (decl == afterFunc || decl == afterReceiver) && (kind == editor.TokenFunction || kind == editor.TokenVariable):
			tokens[i].Kind = editor.TokenFunction
			decl = none
			continue
		}
		decl = none
		if kind == editor.TokenFunction {
			if goBuiltins[word] {
				tokens[i].Kind = editor.TokenType
			} else {
				tokens[i].Kind = editor.TokenMeta
			}
		}
	}
}
