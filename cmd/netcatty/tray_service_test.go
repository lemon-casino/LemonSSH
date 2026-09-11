package main

import (
	"strings"
	"testing"
)

func TestTrayLabelsFollowAppearanceLanguage(t *testing.T) {
	cases := map[string]trayLabels{
		"en":    {Show: "Open Main Window", Settings: "Settings", Quit: "Quit"},
		"es":    {Show: "Abrir ventana principal", Settings: "Ajustes", Quit: "Salir"},
		"ru":    {Show: "Открыть главное окно", Settings: "Настройки", Quit: "Выйти"},
		"zh-CN": {Show: "打开主窗口", Settings: "设置", Quit: "退出"},
		"zh-TW": {Show: "開啟主視窗", Settings: "設定", Quit: "結束"},
	}
	for language, want := range cases {
		got := trayLabelsFor(language)
		if got != want {
			t.Fatalf("%s labels = %+v, want %+v", language, got, want)
		}
	}
}

func TestTrayLabelsFallBackToEnglish(t *testing.T) {
	for _, language := range []string{"", "fr", "en-US", "not-a-locale"} {
		if got := trayLabelsFor(language); got != trayLabelsFor("en") {
			t.Fatalf("%q labels = %+v, want English fallback", language, got)
		}
	}
}

type fakeWindow struct {
	minimised bool
	calls     []string
}

func (w *fakeWindow) IsMinimised() bool { return w.minimised }
func (w *fakeWindow) UnMinimise() {
	w.calls = append(w.calls, "unminimise")
	w.minimised = false
}
func (w *fakeWindow) Show()  { w.calls = append(w.calls, "show") }
func (w *fakeWindow) Focus() { w.calls = append(w.calls, "focus") }

func TestRestoreMainWindowUnminimisesBeforeShow(t *testing.T) {
	win := &fakeWindow{minimised: true}
	restoreMainWindow(win)
	if got := strings.Join(win.calls, ","); got != "unminimise,show,focus" {
		t.Fatalf("restore minimised window = %q, want unminimise,show,focus", got)
	}
	if win.minimised {
		t.Fatal("window must no longer be minimised")
	}
}

func TestRestoreMainWindowShowsVisibleWindow(t *testing.T) {
	win := &fakeWindow{}
	restoreMainWindow(win)
	if got := strings.Join(win.calls, ","); got != "show,focus" {
		t.Fatalf("restore visible window = %q, want show,focus", got)
	}
}

func TestTrayQuitStopsTheApplication(t *testing.T) {
	calls := 0
	service := &TrayService{
		app:  nil,
		quit: func() { calls++ },
	}
	service.Quit()
	if calls != 1 {
		t.Fatalf("Quit calls = %d, want 1", calls)
	}
}

func TestNormalizeTrayLanguageMatchesRendererLocales(t *testing.T) {
	cases := map[string]string{
		"en":      "en",
		"en-US":   "en",
		"es":      "es",
		"es-419":  "es",
		"ru":      "ru",
		"zh-CN":   "zh-CN",
		"zh-TW":   "zh-TW",
		"zh":      "zh-CN",
		"fr":      "en",
		"":        "en",
		"garbage": "en",
	}
	for input, want := range cases {
		if got := normalizeTrayLanguage(input); got != want {
			t.Fatalf("normalizeTrayLanguage(%q) = %q, want %q", input, got, want)
		}
	}
}
