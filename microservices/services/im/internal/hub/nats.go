package hub

import (
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"
)

const (
	convSubjectPrefix = "im.conv."
	// 按租户的坐席广播：im.agents.<tenant_id> 让跨租户隔离
	// 在实例间依然成立。
	agentsSubjectPrefix = "im.agents."
)

// ConnectNATS 将两个 hub 跨实例桥接（设计文档 §7.5）：Publish
// 发往 NATS，每个实例（含发布者自身）收到后在本地 WS 订阅者中分发。
// 返回连接句柄供关闭使用。
func ConnectNATS(url string, h *Hub, ah *AgentHub) (*nats.Conn, error) {
	nc, err := nats.Connect(url,
		nats.Name("im-service"),
		nats.MaxReconnects(-1), // 永远重试
	)
	if err != nil {
		return nil, err
	}
	if _, err := nc.Subscribe(convSubjectPrefix+"*", func(m *nats.Msg) {
		pid := strings.TrimPrefix(m.Subject, convSubjectPrefix)
		h.Dispatch(pid, m.Data)
	}); err != nil {
		nc.Close()
		return nil, err
	}
	if _, err := nc.Subscribe(agentsSubjectPrefix+"*", func(m *nats.Msg) {
		tid, err := strconv.ParseUint(strings.TrimPrefix(m.Subject, agentsSubjectPrefix), 10, 64)
		if err != nil {
			return // 畸形 subject：丢弃而不是跨租户错误路由
		}
		ah.Dispatch(tid, m.Data)
	}); err != nil {
		nc.Close()
		return nil, err
	}
	h.SetRemote(func(pid string, b []byte) error {
		return nc.Publish(convSubjectPrefix+pid, b)
	})
	ah.SetRemote(func(tenantID uint64, b []byte) error {
		return nc.Publish(agentsSubjectPrefix+strconv.FormatUint(tenantID, 10), b)
	})
	return nc, nil
}
