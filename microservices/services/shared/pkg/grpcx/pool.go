package grpcx

import (
	"context"
	"sync"

	"google.golang.org/grpc"
)

// ConnPool 缓存到目标服务的 gRPC 连接（进程内复用）。
//
// 背景：Phase 3/4 客户端此前每调用一次 Dial+Close，产生 Consul 查询 + TCP +
// 握手开销；高频路径（cc 每通识客、notify 每消息、im 每轮 RAG）被放大。
// Swarm 滚动部署会换容器 IP，故调用方在 Unavailable/瞬时故障时调 Invalidate
// 让下次 Get 重拨，避免陈旧连接长驻导致 gRPC 永久失效。
type ConnPool struct {
	service string
	dial    func(ctx context.Context) (*grpc.ClientConn, error)

	mu   sync.Mutex
	conn *grpc.ClientConn
}

// NewConnPool 创建连接池。resolver 为 nil 时 Get 返回错误（调用方回退 HTTP）。
func NewConnPool(r *Resolver, service string) *ConnPool {
	return &ConnPool{service: service, dial: func(ctx context.Context) (*grpc.ClientConn, error) {
		if r == nil {
			return nil, errNilResolver
		}
		return Dial(ctx, r, service)
	}}
}

// newConnPoolWithDial 测试专用：注入拨号函数。
func newConnPoolWithDial(service string, dial func(ctx context.Context) (*grpc.ClientConn, error)) *ConnPool {
	return &ConnPool{service: service, dial: dial}
}

// Get 返回缓存连接；无则拨号并缓存。并发安全，同一时刻只拨号一次。
func (p *ConnPool) Get(ctx context.Context) (*grpc.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		return p.conn, nil
	}
	conn, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	p.conn = conn
	return conn, nil
}

// Invalidate 清空缓存连接（调用方在 RPC 失败且判定为瞬时/陈旧时调用）。
// 已关闭旧连接，下次 Get 重拨。
func (p *ConnPool) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

var errNilResolver = &nilResolverError{}

type nilResolverError struct{}

func (*nilResolverError) Error() string { return "grpcx: no consul resolver" }
