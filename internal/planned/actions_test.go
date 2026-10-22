package planned

import "testing"

func TestListActions(t *testing.T) {
	infos := ListActions()
	got := make(map[string]bool, len(infos))
	for _, a := range infos {
		got[a.Name] = true
	}
	want := []string{"backup", "traverse", "webhook"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("action %q not registered; actions=%v", w, infos)
		}
		if !HasAction(w) {
			t.Errorf("HasAction(%q) = false", w)
		}
	}
	if len(infos) != len(want) {
		t.Errorf("registered %d actions, want %d: %v", len(infos), len(want), infos)
	}
}
