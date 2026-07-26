package system

import (
	"strings"
	"testing"
)

func TestPatchAfterUniqueAnchor(t *testing.T) {
	got, err := patchAfterUniqueAnchor("before\nanchor\nafter\n", "anchor", "\ninserted")
	if err != nil {
		t.Fatalf("patchAfterUniqueAnchor: %v", err)
	}
	if got != "before\nanchor\ninserted\nafter\n" {
		t.Fatalf("patched content = %q", got)
	}
}

func TestPatchAfterUniqueAnchorRejectsMissingOrDuplicateAnchor(t *testing.T) {
	for name, source := range map[string]string{
		"missing":   "before\nafter\n",
		"duplicate": "anchor\nanchor\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := patchAfterUniqueAnchor(source, "anchor", "\ninserted")
			if err == nil || !strings.Contains(err.Error(), "1") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestUnifiedDiffContainsOldAndNewLines(t *testing.T) {
	diff := unifiedDiff("example.go", "before\nold\nafter\n", "before\nnew\nafter\n")
	for _, want := range []string{"--- a/example.go", "+++ b/example.go", "-old", "+new"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff missing %q:\n%s", want, diff)
		}
	}
}
