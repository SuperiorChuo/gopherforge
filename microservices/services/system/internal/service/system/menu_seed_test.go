package system

import "testing"

func TestDefaultMenusRetainPublicTracingEntry(t *testing.T) {
	menus := DefaultMenus()
	for _, menu := range menus {
		if menu.Name != "monitor-jaeger" {
			continue
		}
		if menu.Path != "/monitor/jaeger" || menu.Component != "monitor/jaeger/index" {
			t.Fatalf("unexpected tracing entry: %+v", menu)
		}
		for _, parent := range menus {
			if parent.ID == menu.ParentID && parent.Name == "monitor" {
				return
			}
		}
		t.Fatal("tracing entry is not under monitor")
	}
	t.Fatal("public tracing entry missing")
}

func TestDefaultMenusHaveUniqueIDs(t *testing.T) {
	seen := make(map[uint]string)
	for _, menu := range DefaultMenus() {
		if previous, exists := seen[menu.ID]; exists {
			t.Fatalf("menu id %d is shared by %q and %q", menu.ID, previous, menu.Path)
		}
		seen[menu.ID] = menu.Path
	}
}
