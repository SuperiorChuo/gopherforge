// Package grpcx 服务发现与 gRPC 基建（Phase 1 真分布式骨架）。
//
// Resolver 用 Consul 解析服务名 → 健康实例地址；Register 向 Consul 注册本服务。
// server.go / client.go 封装带 OTel 传播的 gRPC 收发端。
package grpcx

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/go-admin-kit/services/shared/pkg/envsecret"
)

// ConsulAddr 默认 Consul 地址（go-admin-kit-net 上容器名）。
const ConsulAddr = "go-admin-kit-consul:8500"

const defaultCacheTTL = 10 * time.Second

type Resolver struct {
	client    *consulapi.Client
	cacheMu   sync.RWMutex
	cache     map[string]resolverCacheEntry
	cacheTTL  time.Duration
}

type resolverCacheEntry struct {
	addresses []string
	expires   time.Time
}

// resolveConsulAddr 空值回落默认；CONSUL_HTTP_ADDR 覆盖默认地址（显式传入非默认则保留）。
func resolveConsulAddr(consulAddr string) string {
	if consulAddr == "" {
		consulAddr = ConsulAddr
	}
	if v := os.Getenv("CONSUL_HTTP_ADDR"); v != "" && consulAddr == ConsulAddr {
		return v
	}
	return consulAddr
}

// consulToken 读 Consul ACL token：优先 Swarm secret，再 CONSUL_HTTP_TOKEN env。
func consulToken() string {
	return envsecret.Get("CONSUL_HTTP_TOKEN", "")
}

func newConsulClient(consulAddr string) (*consulapi.Client, error) {
	cfg := consulapi.DefaultConfig()
	cfg.Address = resolveConsulAddr(consulAddr)
	if token := consulToken(); token != "" {
		cfg.Token = token
	}
	return consulapi.NewClient(cfg)
}

func NewResolver(consulAddr string) (*Resolver, error) {
	client, err := newConsulClient(consulAddr)
	if err != nil {
		return nil, fmt.Errorf("grpcx: 创建 Consul 客户端: %w", err)
	}
	return &Resolver{
		client:   client,
		cache:    make(map[string]resolverCacheEntry),
		cacheTTL: defaultCacheTTL,
	}, nil
}

func (r *Resolver) Resolve(ctx context.Context, serviceName string) (string, error) {
	if addr, ok := r.lookupCache(serviceName); ok {
		return addr, nil
	}
	addr, err := r.resolveFromConsul(ctx, serviceName)
	if err != nil {
		return "", err
	}
	return addr, nil
}

func (r *Resolver) lookupCache(serviceName string) (string, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	entry, ok := r.cache[serviceName]
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}
	if len(entry.addresses) == 0 {
		return "", false
	}
	return entry.addresses[rand.Intn(len(entry.addresses))], true
}

func (r *Resolver) resolveFromConsul(ctx context.Context, serviceName string) (string, error) {
	opts := &consulapi.QueryOptions{}
	opts = opts.WithContext(ctx)
	entries, _, err := r.client.Health().Service(serviceName, "", true, opts)
	if err != nil {
		return "", fmt.Errorf("grpcx: Consul 解析 %s: %w", serviceName, err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("grpcx: 服务 %s 无健康实例", serviceName)
	}
	addresses := make([]string, 0, len(entries))
	for _, e := range entries {
		addr := e.Service.Address
		if addr == "" {
			addr = e.Node.Address
		}
		addresses = append(addresses, fmt.Sprintf("%s:%d", addr, e.Service.Port))
	}
	r.cacheMu.Lock()
	r.cache[serviceName] = resolverCacheEntry{
		addresses: addresses,
		expires:   time.Now().Add(r.cacheTTL),
	}
	r.cacheMu.Unlock()
	return addresses[rand.Intn(len(addresses))], nil
}

func (r *Resolver) Invalidate(serviceName string) {
	r.cacheMu.Lock()
	delete(r.cache, serviceName)
	r.cacheMu.Unlock()
}

// Instance 注册信息。
type Instance struct {
	ServiceName string
	Host        string
	Port        int
	// 健康检查探活路径；空则不注册 HTTP check。
	HealthPath string
	// 健康检查间隔；默认 10s。
	Interval time.Duration
}

// Register 向 Consul 注册本服务并附带健康检查（TTL/HTTP）。
// 返回注销函数，服务退出时调用（graceful shutdown）。
// 与 NewResolver 共用地址解析与 ACL token（envsecret）。
func Register(consulAddr string, inst Instance) (deregister func(), err error) {
	client, err := newConsulClient(consulAddr)
	if err != nil {
		return nil, fmt.Errorf("grpcx: 创建 Consul 客户端: %w", err)
	}

	interval := inst.Interval
	if interval == 0 {
		interval = 10 * time.Second
	}

	reg := &consulapi.AgentServiceRegistration{
		ID:      fmt.Sprintf("%s-%s:%d", inst.ServiceName, inst.Host, inst.Port),
		Name:    inst.ServiceName,
		Address: inst.Host,
		Port:    inst.Port,
	}
	var checkID string
	if inst.HealthPath != "" {
		reg.Check = &consulapi.AgentServiceCheck{
			HTTP:     fmt.Sprintf("http://%s:%d%s", inst.Host, inst.Port, inst.HealthPath),
			Interval: interval.String(),
			Timeout:  "3s",
		}
	} else {
		// TTL 检查：注册后必须周期 PassTTL，否则 Consul 判定服务不健康。
		reg.Check = &consulapi.AgentServiceCheck{TTL: interval.String()}
	}

	if err := client.Agent().ServiceRegister(reg); err != nil {
		return nil, fmt.Errorf("grpcx: Consul 注册 %s: %w", inst.ServiceName, err)
	}
	if reg.Check != nil && reg.Check.TTL != "" {
		// Consul 的 checkID 形如 service:<serviceID>:ttl
		checkID = fmt.Sprintf("service:%s", reg.ID)
		stop := make(chan struct{})
		go func() {
			t := time.NewTicker(interval / 2)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					_ = client.Agent().PassTTL(checkID, "")
				case <-stop:
					return
				}
			}
		}()
		deregister = func() {
			close(stop)
			_ = client.Agent().ServiceDeregister(reg.ID)
		}
		return deregister, nil
	}
	deregister = func() {
		_ = client.Agent().ServiceDeregister(reg.ID)
	}
	return deregister, nil
}

// LocalIP 返回本容器第一个非回环 IPv4 地址。
// Swarm overlay 单网络下即容器在 go-admin-kit-net 上的 IP，用于 Consul 注册对外可达地址。
func LocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}
