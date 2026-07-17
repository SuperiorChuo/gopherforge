package hub

import (
	"strings"

	"github.com/nats-io/nats.go"
)

const (
	convSubjectPrefix = "im.conv."
	agentsSubject     = "im.agents"
)

// ConnectNATS bridges both hubs across instances (design doc §7.5): Publish
// goes to NATS, every instance (including the publisher) dispatches to its
// local WS subscribers on receipt. Returns the connection for shutdown.
func ConnectNATS(url string, h *Hub, ah *AgentHub) (*nats.Conn, error) {
	nc, err := nats.Connect(url,
		nats.Name("im-service"),
		nats.MaxReconnects(-1), // keep retrying forever
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
	if _, err := nc.Subscribe(agentsSubject, func(m *nats.Msg) {
		ah.Dispatch(m.Data)
	}); err != nil {
		nc.Close()
		return nil, err
	}
	h.SetRemote(func(pid string, b []byte) error {
		return nc.Publish(convSubjectPrefix+pid, b)
	})
	ah.SetRemote(func(b []byte) error {
		return nc.Publish(agentsSubject, b)
	})
	return nc, nil
}
