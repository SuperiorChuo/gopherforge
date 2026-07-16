package bot

import "testing"

func TestWantsHuman(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"转人工", true},
		{"请帮我转人工客服", true},
		{"I need a live agent", true},
		{"你好", false},
		{"订单什么时候到", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := WantsHuman(tc.in); got != tc.want {
			t.Fatalf("WantsHuman(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestStubComplete(t *testing.T) {
	s := NewStub()
	out, err := s.Complete(nil, "", []Message{{Role: "user", Content: "你好"}})
	if err != nil || out == "" {
		t.Fatalf("stub: %v %q", err, out)
	}
}

func TestExtractText(t *testing.T) {
	if ExtractText(`{"text":"hi"}`) != "hi" {
		t.Fatal("extract")
	}
}
