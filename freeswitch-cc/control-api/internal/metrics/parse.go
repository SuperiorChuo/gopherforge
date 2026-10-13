package metrics

import (
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

// parsePipeTable 解析 mod_callcenter 的 |-分隔表格为 map 列表。
// 与 api.parsePipeTable 同源逻辑（跨包无法直接复用私有函数），
// 采集器只依赖表头字段名，格式微调也不至于串列。
func parsePipeTable(raw string) []map[string]string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return nil
	}
	header := strings.Split(strings.TrimRight(lines[0], "\r"), "|")
	var rows []map[string]string
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "+OK") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) != len(header) {
			continue
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			row[strings.TrimSpace(h)] = strings.TrimSpace(cols[i])
		}
		rows = append(rows, row)
	}
	return rows
}

// parseCountTotal 解析 `show channels count` / `show calls count` 输出。
// 典型形如 "12 total." 或 "0 total."；取首个整数。空表/异常回 0,false。
func parseCountTotal(raw string) (int, bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseQueueNames 解析 `callcenter_config queue list` 的队列名清单。
// 表头首列为 name。
func parseQueueNames(raw string) []string {
	var names []string
	for _, row := range parsePipeTable(raw) {
		if n := row["name"]; n != "" {
			// 队列名在 FS 里带 @domain（如 sales@default），指标只留裸名。
			names = append(names, stripDomain(n))
		}
	}
	return names
}

func stripDomain(name string) string {
	if i := strings.IndexByte(name, '@'); i >= 0 {
		return name[:i]
	}
	return name
}

// queueMemberStats 汇总单个队列的排队情况。
type queueMemberStats struct {
	Members         int
	LongestWaitSecs int
	Answered        int // state=Answered 的历史记录数（若输出包含）
	Abandoned       int // state=Abandoned 的历史记录数（若输出包含）
}

// parseQueueMembers 解析 `callcenter_config queue list members <queue>@default`。
// 每行一个 caller，列含 system_epoch（入队时间戳）与 state。
// 等待中的 caller（state=Waiting/Trying）计入 Members 与最长等待；
// Answered/Abandoned 状态则累加各自计数（mod_callcenter 会保留近期终态行）。
// nowUnix 传当前时间戳，便于测试注入固定值。
func parseQueueMembers(raw string, nowUnix int64) queueMemberStats {
	var st queueMemberStats
	for _, row := range parsePipeTable(raw) {
		state := row["state"]
		switch state {
		case "Answered":
			st.Answered++
			continue
		case "Abandoned":
			st.Abandoned++
			continue
		}
		// 其余视为在队等待（Waiting / Trying / Ringing …）
		st.Members++
		epoch := parseInt(row["system_epoch"])
		if epoch <= 0 {
			// 有的构建用 join_epoch/system_epoch 命名差异，退而求其次。
			epoch = parseInt(row["join_epoch"])
		}
		if epoch > 0 && nowUnix >= epoch {
			if w := int(nowUnix - epoch); w > st.LongestWaitSecs {
				st.LongestWaitSecs = w
			}
		}
	}
	return st
}

// parseAgentStatuses 解析 `callcenter_config agent list`，按 status 归组计数。
func parseAgentStatuses(raw string) map[string]int {
	out := map[string]int{}
	for _, row := range parsePipeTable(raw) {
		status := row["status"]
		if status == "" {
			status = "Unknown"
		}
		out[status]++
	}
	return out
}

// gatewayState 保存单个网关的名称与注册状态。
type gatewayState struct {
	Name  string
	State string
}

// sofia xmlstatus gateways 的 XML 结构（只取关心字段）。
type xmlGateways struct {
	XMLName  xml.Name     `xml:"gateways"`
	Gateways []xmlGateway `xml:"gateway"`
}

type xmlGateway struct {
	Name  string `xml:"name"`
	State string `xml:"state"`
}

// parseGatewayStates 解析 `sofia xmlstatus gateways` 的 XML 输出。
// 形如：
//
//	<gateways>
//	  <gateway><name>gw1</name>...<state>REGED</state>...</gateway>
//	  <gateway><name>gw2</name>...<state>NOREG</state>...</gateway>
//	</gateways>
//
// XML 比 `sofia status` 的对齐文本表稳得多（列宽/制表符易错位）。
// 无网关或解析失败时回 nil。
func parseGatewayStates(raw string) []gatewayState {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var doc xmlGateways
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}
	out := make([]gatewayState, 0, len(doc.Gateways))
	for _, g := range doc.Gateways {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}
		out = append(out, gatewayState{Name: name, State: strings.TrimSpace(g.State)})
	}
	return out
}

// gatewayUp 把注册状态映射为指标值：REGED/NOREG=1（进程存活），其余=0。
// NOREG 也算 1：非注册型（outbound-only）网关正常态就是 NOREG。
func gatewayUp(state string) float64 {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "REGED", "NOREG":
		return 1
	default:
		return 0
	}
}

func parseInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// nowUnix 便于采集器与测试共用当前秒级时间戳。
func nowUnix() int64 { return time.Now().Unix() }
