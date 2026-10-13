package sensitive

import "testing"

func TestMatcherMaskBasic(t *testing.T) {
	m := NewMatcher([]Entry{{Word: "辣鸡"}, {Word: "scam"}})
	masked, blocked, hits := m.Apply("你们真是辣鸡平台 SCAM 吗")
	if blocked {
		t.Fatal("mask words must not block")
	}
	if masked != "你们真是**平台 **** 吗" {
		t.Fatalf("masked = %q", masked)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestMatcherBlockWins(t *testing.T) {
	m := NewMatcher([]Entry{{Word: "打码词"}, {Word: "违禁品", Action: ActionBlock}})
	masked, blocked, hits := m.Apply("这里有打码词也有违禁品")
	if !blocked {
		t.Fatal("expected blocked")
	}
	if masked != "这里有打码词也有违禁品" {
		t.Fatalf("blocked text must stay unchanged, got %q", masked)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestMatcherLongestMatch(t *testing.T) {
	// 同前缀取最长：命中「敏感词汇」而非「敏感词」
	m := NewMatcher([]Entry{{Word: "敏感词"}, {Word: "敏感词汇"}})
	masked, _, hits := m.Apply("这是敏感词汇测试")
	if masked != "这是****测试" {
		t.Fatalf("masked = %q", masked)
	}
	if len(hits) != 1 || hits[0].Word != "敏感词汇" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestMatcherCaseInsensitive(t *testing.T) {
	m := NewMatcher([]Entry{{Word: "FooBar"}})
	masked, _, _ := m.Apply("say fooBAR now")
	if masked != "say ****** now" {
		t.Fatalf("masked = %q", masked)
	}
}

func TestMatcherNoHit(t *testing.T) {
	m := NewMatcher([]Entry{{Word: "违禁"}})
	masked, blocked, hits := m.Apply("完全正常的内容")
	if blocked || hits != nil || masked != "完全正常的内容" {
		t.Fatalf("unexpected: %q %v %+v", masked, blocked, hits)
	}
}

func TestMatcherDuplicateBlockUpgrade(t *testing.T) {
	// 同词先 mask 后 block：升级为 block
	m := NewMatcher([]Entry{{Word: "重复词"}, {Word: "重复词", Action: ActionBlock}})
	_, blocked, _ := m.Apply("包含重复词")
	if !blocked {
		t.Fatal("expected block after upgrade")
	}
}

func TestMatcherNilSafe(t *testing.T) {
	var m *Matcher
	masked, blocked, hits := m.Apply("anything")
	if masked != "anything" || blocked || hits != nil {
		t.Fatal("nil matcher must be a no-op")
	}
	if m.Size() != 0 {
		t.Fatal("nil size")
	}
}
