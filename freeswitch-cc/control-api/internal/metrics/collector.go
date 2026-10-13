package metrics

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// APIClient 是采集器需要的 ESL 能力子集（*esl.Client 已满足）。
type APIClient interface {
	API(cmd string) (string, error)
}

// Collector 每 interval 拉一轮 ESL，把结果写进 Metrics。
type Collector struct {
	esl     APIClient
	m       *Metrics
	inter   time.Duration
	nowUnix func() int64 // 便于测试注入
}

// NewCollector 建采集器；interval<=0 时用默认 10s。
func NewCollector(escl APIClient, m *Metrics, interval time.Duration) *Collector {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &Collector{esl: escl, m: m, inter: interval, nowUnix: nowUnix}
}

// Run 阻塞采集直到 ctx 取消；先立即跑一轮再进入定时循环。
func (c *Collector) Run(ctx context.Context) {
	c.collectOnce()
	t := time.NewTicker(c.inter)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.collectOnce()
		}
	}
}

// collectOnce 拉一轮 ESL；只要有一条命令成功就算 ESL 存活。
// 全部失败时 fscc_esl_up=0，且不改动任何业务 gauge（保留上轮值）。
func (c *Collector) collectOnce() {
	anyOK := false
	call := func(cmd string) (string, bool) {
		out, err := c.esl.API(cmd)
		if err != nil {
			log.Printf("metrics: esl %q: %v", cmd, err)
			return "", false
		}
		anyOK = true
		return out, true
	}

	// 全局并发
	if out, ok := call("show channels count"); ok {
		if n, valid := parseCountTotal(out); valid {
			c.m.ChannelsTotal.Set(float64(n))
		}
	}
	if out, ok := call("show calls count"); ok {
		if n, valid := parseCountTotal(out); valid {
			c.m.CallsTotal.Set(float64(n))
		}
	}

	// 队列维度
	if out, ok := call("callcenter_config queue list"); ok {
		now := c.nowUnix()
		for _, q := range parseQueueNames(out) {
			target := q + "@default"
			if mraw, mok := call(fmt.Sprintf("callcenter_config queue list members %s", target)); mok {
				st := parseQueueMembers(mraw, now)
				c.m.QueueMembers.WithLabelValues(q).Set(float64(st.Members))
				c.m.QueueLongestWaitSecs.WithLabelValues(q).Set(float64(st.LongestWaitSecs))
				c.m.QueueCallsAnswered.WithLabelValues(q).Set(float64(st.Answered))
				c.m.QueueCallsAbandoned.WithLabelValues(q).Set(float64(st.Abandoned))
			}
		}
	}

	// 坐席状态分布：先清零已知状态再回填，避免状态迁移后残留旧标签。
	if out, ok := call("callcenter_config agent list"); ok {
		c.m.Agents.Reset()
		for status, n := range parseAgentStatuses(out) {
			c.m.Agents.WithLabelValues(status).Set(float64(n))
		}
	}

	// 网关注册状态。sofia status 聚合视图能一次拿全，逐个 gateway 兜底更稳。
	c.collectGateways(call)

	if anyOK {
		c.m.ESLUp.Set(1)
	} else {
		c.m.ESLUp.Set(0)
	}
}

// collectGateways 解析 `sofia xmlstatus gateways` 的 XML；解析不到网关时不动指标。
func (c *Collector) collectGateways(call func(string) (string, bool)) {
	out, ok := call("sofia xmlstatus gateways")
	if !ok {
		return
	}
	gws := parseGatewayStates(out)
	if len(gws) == 0 {
		return
	}
	c.m.GatewayUp.Reset()
	for _, g := range gws {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}
		c.m.GatewayUp.WithLabelValues(name).Set(gatewayUp(g.State))
	}
}
