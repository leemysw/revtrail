// INPUT: config 里的模型服务地址与密钥、commitai 客户端。
// OUTPUT: 设置 › AI 面板（已存密钥显示首尾字符、中间用圆点）与模型列表拉取。
// POS: 提交信息生成所用模型服务的配置界面。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/commitai"
	"revtrail/internal/theme"
	"revtrail/locales"
)

type aiSettings struct {
	base, key, err string
	// saved is the key saved for the address in use, shown as its head
	// and tail, or "" when there is none.
	saved   string
	models  []string
	loading bool
}

func aiClient(base string) (commitai.Client, error) {
	key, err := mygo.Secrets.Get("commit-ai:" + base)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		err = nil
	}
	return commitai.Client{BaseURL: base, Key: string(key)}, err
}

func (a *App) aiPane(c *ui.Context) {
	s := a.settings.ai
	if s == nil {
		s = &aiSettings{base: a.cfg.AIBaseURL}
		a.settings.ai = s
		if s.base == "" {
			s.base = commitai.DefaultURL
		} else {
			a.fetchAIModels(s, false)
		}
	}
	fieldWidth := func(e ui.Element) ui.Element { return e.WidthPercent(55).MaxWidth(330).Shrink(0) }
	heading(c, locales.AiProvider())
	card(c, func() {
		labeled(c, locales.AiBaseURL(), "")
		fieldWidth(ui.TextInput(c, &s.base)).Label(locales.AiBaseURL()).Disabled(s.loading)
	}, func() {
		labeled(c, "API Key", locales.AiKeyHint())
		key := fieldWidth(ui.TextInput(c, &s.key).Password()).Label("API Key").Disabled(s.loading)
		if hint := a.savedKeyHint(s); hint != "" {
			key.Placeholder(hint)
		}
	}, func() {
		ui.Box(c).Grow(1)
		if s.loading {
			ui.Spinner(c)
		}
		ui.Button(c, locales.AiLoadModels()).Disabled(s.loading).OnClick(func() { a.fetchAIModels(s, true) })
	})
	if s.err != "" {
		ui.Text(c, s.err).Wrap().TextColor(theme.Of(c).Err)
	}
	if a.cfg.AIBaseURL == "" || strings.TrimRight(strings.TrimSpace(s.base), "/") != a.cfg.AIBaseURL {
		return
	}
	heading(c, locales.AiCommitModel())
	card(c, func() {
		labeled(c, locales.AiModel(), locales.AiModelHint())
		fieldWidth(ui.Row(c)).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.TextInput(c, &a.cfg.AIModel).Grow(1).MinWidth(0).Label(locales.AiModel()).Changed() {
				a.settingsChanged()
			}
			if len(s.models) > 0 {
				ui.Button(c, locales.MsgChoose()).Menu(func(m *ui.Menu) {
					for _, id := range s.models {
						if m.Item(id).Checked(a.cfg.AIModel == id).Chosen() {
							a.cfg.AIModel = id
							a.settingsChanged()
						}
					}
				})
			}
		})
	})
}

func (a *App) fetchAIModels(s *aiSettings, save bool) {
	base, err := commitai.BaseURL(s.base)
	if err != nil {
		s.err = err.Error()
		return
	}
	key := strings.TrimSpace(s.key)
	if save && key != "" {
		if err := mygo.Secrets.Set("commit-ai:"+base, []byte(key)); err != nil {
			s.err = err.Error()
			return
		}
		s.key = ""
	}
	if save {
		if base != a.cfg.AIBaseURL {
			a.cfg.AIModel = ""
			s.models = nil
		}
		a.cfg.AIBaseURL = base
		a.settingsChanged()
	}
	s.loading, s.err = true, ""
	go func() {
		client, err := aiClient(base)
		var models []string
		saved := maskKey(client.Key)
		if err == nil {
			models, err = client.Models(context.Background())
		}
		a.post.Post(func() {
			if a.settings.ai != s {
				return
			}
			s.loading = false
			if base == a.cfg.AIBaseURL {
				s.saved = saved
			}
			if base == a.cfg.AIBaseURL {
				s.models = models
				if err != nil {
					s.err = err.Error()
				} else if len(models) == 0 {
					s.err = locales.AiNoModels()
				} else if len(models) == 1 && a.cfg.AIModel == "" {
					a.cfg.AIModel = models[0]
					a.settingsChanged()
				}
			}
			a.host.RefreshSettings()
		})
	}()
}

// savedKeyHint is what the empty key field shows: the saved key, masked,
// while the address is the one it was saved for.
func (a *App) savedKeyHint(s *aiSettings) string {
	if strings.TrimRight(strings.TrimSpace(s.base), "/") != a.cfg.AIBaseURL {
		return ""
	}
	return s.saved
}

// maskKey shows a key as its first and last four characters with dots
// for the rest, as many as it hides up to a field's width; a key too
// short to show any of is dots alone.
func maskKey(key string) string {
	r := []rune(strings.TrimSpace(key))
	switch {
	case len(r) == 0:
		return ""
	case len(r) <= 12:
		return strings.Repeat("•", len(r))
	}
	return string(r[:4]) + strings.Repeat("•", min(len(r)-8, 32)) + string(r[len(r)-4:])
}
