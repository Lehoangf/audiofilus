package ui

import (
	"testing"
)

func TestNowMenuHasEffectsItems(t *testing.T) {
	app := &App{}
	app.openNowMenu()
	if app.menu == nil {
		t.Fatalf("expected menu to be open")
	}
	foundViz, foundProg := false, false
	for _, item := range app.menu.items {
		if item.label == T("set_now_viz") {
			foundViz = true
		}
		if item.label == T("set_prog_style") {
			foundProg = true
		}
	}
	if !foundViz || !foundProg {
		t.Errorf("missing effects items in now menu: foundViz=%v, foundProg=%v", foundViz, foundProg)
	}
}
