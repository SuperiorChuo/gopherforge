// Package metrics 暴露 FreeSWITCH 呼叫中心的 Prometheus 指标。
// 采集器（collector.go）每 10s 拉一轮 ESL，把结果写进这些 gauge；
// /metrics 由 promhttp 直接读当前值（无副作用）。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 持有全部 gauge。用独立 Registry 而非默认全局，避免测试/多实例串味，
// 也方便只暴露本项目关心的指标（不带 go_* / process_* 噪声，除非显式注册）。
type Metrics struct {
	reg *prometheus.Registry

	// 全局并发
	ChannelsTotal prometheus.Gauge
	CallsTotal    prometheus.Gauge

	// 队列维度（label: queue）
	QueueMembers         *prometheus.GaugeVec
	QueueLongestWaitSecs *prometheus.GaugeVec
	QueueCallsAnswered   *prometheus.GaugeVec
	QueueCallsAbandoned  *prometheus.GaugeVec

	// 坐席状态分布（label: status，如 Available / On Break / Logged Out）
	Agents *prometheus.GaugeVec

	// 网关注册状态（label: gateway；REGED/NOREG=1，其余=0）
	GatewayUp *prometheus.GaugeVec

	// ESL 采集健康：一轮全部失败=0，否则=1
	ESLUp prometheus.Gauge
}

// New 建一套指标并注册到自带 Registry。
func New() *Metrics {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)
	return &Metrics{
		reg: reg,
		ChannelsTotal: factory.NewGauge(prometheus.GaugeOpts{
			Name: "fscc_channels_total",
			Help: "当前 FreeSWITCH 活动通道数（show channels count）。",
		}),
		CallsTotal: factory.NewGauge(prometheus.GaugeOpts{
			Name: "fscc_calls_total",
			Help: "当前 FreeSWITCH 活动呼叫数（show calls count）。",
		}),
		QueueMembers: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fscc_queue_members",
			Help: "各队列当前排队人数（callcenter_config queue list members）。",
		}, []string{"queue"}),
		QueueLongestWaitSecs: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fscc_queue_longest_wait_seconds",
			Help: "各队列当前最长等待时长（秒）。",
		}, []string{"queue"}),
		QueueCallsAnswered: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fscc_queue_calls_answered_total",
			Help: "各队列累计接通数（来自 callcenter_config queue count，非严格单调）。",
		}, []string{"queue"}),
		QueueCallsAbandoned: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fscc_queue_calls_abandoned_total",
			Help: "各队列累计放弃数（来自 callcenter_config queue count，非严格单调）。",
		}, []string{"queue"}),
		Agents: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fscc_agents",
			Help: "按状态统计的坐席数（Available / On Break / Logged Out 等）。",
		}, []string{"status"}),
		GatewayUp: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fscc_gateway_up",
			Help: "SIP 网关注册状态：REGED/NOREG=1（进程在跑），其余=0。",
		}, []string{"gateway"}),
		ESLUp: factory.NewGauge(prometheus.GaugeOpts{
			Name: "fscc_esl_up",
			Help: "最近一轮 ESL 采集是否成功：1=成功，0=全部失败（保留上轮指标值）。",
		}),
	}
}

// Registry 供 promhttp.HandlerFor 使用。
func (m *Metrics) Registry() *prometheus.Registry {
	return m.reg
}
