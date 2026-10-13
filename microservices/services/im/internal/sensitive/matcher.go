// Package sensitive 提供敏感词匹配（rune 级 trie，大小写不敏感、贪心最长匹配）
// 与按租户的词库缓存。词库来自 DB（控制台可维护，写入后热生效），
// env IM_SENSITIVE_WORDS 兜底词仍由 api.Server.maskSensitive 叠加处理。
package sensitive

import (
	"strings"
	"unicode"
)

const (
	ActionMask  = "mask"
	ActionBlock = "block"
)

// Entry 是一个词及其命中动作。
type Entry struct {
	Word   string
	Action string // mask | block
}

// Hit 是一次匹配命中，坐标为 rune 偏移。
type Hit struct {
	Word   string `json:"word"`
	Action string `json:"action"`
	Start  int    `json:"start"` // rune 偏移，含起点
	End    int    `json:"end"`   // rune 偏移，不含终点
}

type trieNode struct {
	children map[rune]*trieNode
	// end != nil 表示一个完整词；value 保存规范条目。
	end *Entry
}

// Matcher 是不可变词匹配器；可安全并发使用。
type Matcher struct {
	root *trieNode
	size int
}

// NewMatcher 根据条目构建 trie。空/纯空白词会被跳过；
// 重复词保留第一个条目（冲突时 block 优先于 mask）。
func NewMatcher(entries []Entry) *Matcher {
	m := &Matcher{root: &trieNode{}}
	for i := range entries {
		e := entries[i]
		word := strings.TrimSpace(e.Word)
		if word == "" {
			continue
		}
		if e.Action != ActionBlock {
			e.Action = ActionMask
		}
		node := m.root
		for _, r := range strings.ToLower(word) {
			if node.children == nil {
				node.children = map[rune]*trieNode{}
			}
			next := node.children[r]
			if next == nil {
				next = &trieNode{}
				node.children[r] = next
			}
			node = next
		}
		if node.end == nil {
			node.end = &Entry{Word: word, Action: e.Action}
			m.size++
		} else if e.Action == ActionBlock {
			node.end.Action = ActionBlock
		}
	}
	return m
}

// Size 返回已加载的去重词数量。
func (m *Matcher) Size() int {
	if m == nil {
		return 0
	}
	return m.size
}

// Match 扫描文本并返回不重叠的命中（贪心最长匹配，
// 从左到右扫描），大小写不敏感。
func (m *Matcher) Match(text string) []Hit {
	if m == nil || m.size == 0 || text == "" {
		return nil
	}
	runes := []rune(text)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	var hits []Hit
	for i := 0; i < len(lower); {
		node := m.root
		var match *Entry
		matchEnd := i
		for j := i; j < len(lower); j++ {
			next := node.children[lower[j]]
			if next == nil {
				break
			}
			node = next
			if node.end != nil {
				match = node.end
				matchEnd = j + 1
			}
		}
		if match == nil {
			i++
			continue
		}
		hits = append(hits, Hit{Word: match.Word, Action: match.Action, Start: i, End: matchEnd})
		i = matchEnd
	}
	return hits
}

// Apply 将 mask 动作的命中以 '*' 打码（每个 rune 一个），并报告是否命中
// block 动作的词。命中 block 时原文本原样返回——
// 调用方应拒绝该消息而不是存储它。
func (m *Matcher) Apply(text string) (masked string, blocked bool, hits []Hit) {
	hits = m.Match(text)
	if len(hits) == 0 {
		return text, false, nil
	}
	for _, h := range hits {
		if h.Action == ActionBlock {
			return text, true, hits
		}
	}
	runes := []rune(text)
	for _, h := range hits {
		for i := h.Start; i < h.End && i < len(runes); i++ {
			runes[i] = '*'
		}
	}
	return string(runes), false, hits
}
