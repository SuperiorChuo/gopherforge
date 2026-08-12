package edgecert

import "testing"

func TestChallengeStore(t *testing.T) {
	putChallenge("tok1", "auth1")
	v, ok := LookupChallenge("tok1")
	if !ok || v != "auth1" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
	deleteChallenge("tok1")
	if _, ok := LookupChallenge("tok1"); ok {
		t.Fatal("expected deleted")
	}
}
