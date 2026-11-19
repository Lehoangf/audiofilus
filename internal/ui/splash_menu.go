package ui

import (
	"path/filepath"

	"github.com/juanvillacortac/audiofilus/internal/sfx"
)

var splashColorKeys = []string{
	"splash_col_default",
	"splash_col_now",
	"splash_col_theme",
	"splash_col_cyber",
	"splash_col_matrix",
	"splash_col_amber",
	"splash_col_silver",
}

func (a *App) splashValue() string {
	if a.cfg.UI.SplashWord != "" {
		return a.cfg.UI.SplashWord
	}
	idx := a.cfg.UI.SplashColor % len(splashColorKeys)
	return T(splashColorKeys[idx])
}

func (a *App) openSplashMenu() {
	m := &menu{
		title: T("set_splash"),
		items: []menuItem{
			{
				label: T("splash_preview"),
				icon:  "palette",
				do: func() {
					a.menu = nil
					a.splashing = true
					a.splashT = 0
					a.playSound(sfx.Splash)
				},
			},
			{
				label: T("splash_color"),
				icon:  "palette",
				value: func() string {
					idx := a.cfg.UI.SplashColor % len(splashColorKeys)
					return T(splashColorKeys[idx])
				},
				do: func() {
					a.cfg.UI.SplashColor = (a.cfg.UI.SplashColor + 1) % len(splashColorKeys)
					a.saveConfig()
				},
			},
			{
				label: T("splash_word"),
				icon:  "pencil",
				value: func() string {
					if a.cfg.UI.SplashWord != "" {
						return a.cfg.UI.SplashWord
					}
					return "audiofilus"
				},
				do: func() {
					m := a.menu
					a.openRawPrompt(T("splash_word"), a.cfg.UI.SplashWord, false, func(s string) {
						a.cfg.UI.SplashWord = s
						a.saveConfig()
						a.openSplashMenu()
					})
					if a.prompt != nil {
						a.prompt.back = m
					}
				},
			},
			{
				label: T("splash_by"),
				icon:  "user",
				value: func() string {
					if a.cfg.UI.SplashBy != "" {
						return a.cfg.UI.SplashBy
					}
					return "juanvillacortac"
				},
				do: func() {
					m := a.menu
					a.openRawPrompt(T("splash_by"), a.cfg.UI.SplashBy, false, func(s string) {
						a.cfg.UI.SplashBy = s
						a.saveConfig()
						a.openSplashMenu()
					})
					if a.prompt != nil {
						a.prompt.back = m
					}
				},
			},
			{
				label: T("splash_img"),
				icon:  "folder",
				value: func() string {
					if a.cfg.UI.SplashImage != "" {
						return filepath.Base(a.cfg.UI.SplashImage)
					}
					return T("none")
				},
				do: func() {
					a.picker = newPicker("")
					a.pickFor = pickSplashImg
					a.mode = modePicker
				},
			},
			{
				label: T("splash_clear_img"),
				icon:  "trash",
				do: func() {
					a.cfg.UI.SplashImage = ""
					a.splashCustomImg = nil
					a.saveConfig()
					a.flash(T("splash_cleared"))
					a.openSplashMenu()
				},
			},
			{
				label: T("splash_always"),
				icon:  "clock",
				active: func() bool {
					return a.cfg.UI.SplashAlways
				},
				value: func() string {
					return onOffLabel(a.cfg.UI.SplashAlways)
				},
				do: func() {
					a.cfg.UI.SplashAlways = !a.cfg.UI.SplashAlways
					a.saveConfig()
				},
			},
			{
				label: T("splash_reset"),
				icon:  "repeat",
				do: func() {
					a.cfg.UI.SplashWord = ""
					a.cfg.UI.SplashBy = ""
					a.cfg.UI.SplashImage = ""
					a.cfg.UI.SplashColor = 0
					a.cfg.UI.SplashAlways = false
					a.splashCustomImg = nil
					a.saveConfig()
					a.flash(T("splash_reset_done"))
					a.openSplashMenu()
				},
			},
		},
	}
	a.menu = m
}
