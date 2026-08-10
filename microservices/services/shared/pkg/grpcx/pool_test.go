package grpcx

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// realConn 返回一个可 Close 的真实 gRPC 连接（经 bufconn，不真连网络）。
func realConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("new conn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestConnPoolCachesConn(t *testing.T) {
	var dials int
	pool := newConnPoolWithDial("test-service", func(context.Context) (*grpc.ClientConn, error) {
		dials++
		return &grpc.ClientConn{}, nil
	})
	ctx := context.Background()

	c1, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	c2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c1 != c2 {
		t.Fatal("缓存应返回同一连接实例")
	}
	if dials != 1 {
		t.Fatalf("两次 Get 应只拨号 1 次，实际 %d", dials)
	}
}

func TestConnPoolInvalidateRedials(t *testing.T) {
	var dials int
	pool := newConnPoolWithDial("test-service", func(context.Context) (*grpc.ClientConn, error) {
		dials++
		return realConn(t), nil
	})
	ctx := context.Background()

	_, _ = pool.Get(ctx)
	pool.Invalidate()
	c2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get after invalidate: %v", err)
	}
	if c2 == nil {
		t.Fatal("重拨后应返回新连接")
	}
	if dials != 2 {
		t.Fatalf("失效后应重拨，实际拨号 %d 次", dials)
	}
}

func TestConnPoolNilResolver(t *testing.T) {
	pool := NewConnPool(nil, "no-such")
	_, err := pool.Get(context.Background())
	if err == nil {
		t.Fatal("nil resolver 应返回错误")
	}
}

func TestConnPoolDialErrorNotCached(t *testing.T) {
	var dials int
	pool := newConnPoolWithDial("test-service", func(context.Context) (*grpc.ClientConn, error) {
		dials++
		return nil, errors.New("dial failed")
	})
	if _, err := pool.Get(context.Background()); err == nil {
		t.Fatal("拨号失败应返回错误")
	}
	if _, err := pool.Get(context.Background()); err == nil {
		t.Fatal("拨号失败不缓存，第二次应重拨并再失败")
	}
	if dials != 2 {
		t.Fatalf("失败后不缓存应每次重拨，实际 %d", dials)
	}
}

// 并发 Get 只拨号一次（互斥串行化）。
func TestConnPoolConcurrentGet(t *testing.T) {
	var dials int
	var mu sync.Mutex
	pool := newConnPoolWithDial("test-service", func(context.Context) (*grpc.ClientConn, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return &grpc.ClientConn{}, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pool.Get(context.Background())
		}()
	}
	wg.Wait()
	mu.Lock()
	n := dials
	mu.Unlock()
	if n != 1 {
		t.Fatalf("并发 Get 应只拨号 1 次，实际 %d", n)
	}
}
