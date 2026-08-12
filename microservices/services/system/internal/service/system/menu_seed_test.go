package system

import "testing"

func TestDefaultMenusHaveUniqueIDs(t *testing.T) {
	seen := make(map[uint]string)
	for _, menu := range DefaultMenus() {
		if previous, exists := seen[menu.ID]; exists {
			t.Fatalf("menu id %d is shared by %q and %q", menu.ID, previous, menu.Path)
		}
		seen[menu.ID] = menu.Path
	}
}
