package api

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
)

// Outbox 把 email/sms 外发从 HTTP 请求路径上摘下来：有界队列 + 固定 worker
// 池异步投递，接口在站内信落库后立即返回。
//
// 设计约束：
//   - 有界：队列满时不阻塞请求、也不无声丢弃——记日志 + Dropped 计数（返回体
//     的 dropped 字段），运维能看见背压。
//   - 可等待：Shutdown 关队列并等 in-flight 投递做完（配合 main 的 graceful
//     shutdown），进程退出不会把已受理的邮件吞掉。
//   - 投递日志批量写：一个 job 的 1~2 条结果一次 INSERT。
type Outbox struct {
	store      *store.Store
	dispatcher *channel.Dispatcher
	jobs       chan outboxJob
	wg         sync.WaitGroup
	timeout    time.Duration

	closeOnce sync.Once
	closed    atomic.Bool

	enqueued atomic.Uint64
	dropped  atomic.Uint64
	sent     atomic.Uint64
}

// outboxJob 是一个收件人的外发任务（渲染结果整批共享，逐人只带联系方式）。
type outboxJob struct {
	tenantID  uint64
	userID    uint64
	messageID uint64
	code      string
	refType   string
	refID     string
	channels  []string
	contact   channel.Contact
	payload   channel.Payload
}

// OutboxOptions 控制并发与队列容量；零值走缺省（4 workers / 1024 队列）。
type OutboxOptions struct {
	Workers   int
	QueueSize int
	// Timeout 单个 job 的投递超时（SMTP/短信外呼）。
	Timeout time.Duration
}

const (
	defaultOutboxWorkers = 4
	defaultOutboxQueue   = 1024
	defaultOutboxTimeout = 20 * time.Second
)

// NewOutbox 启动 worker 池。dispatcher 为 nil 时返回 nil（外发关闭）。
func NewOutbox(st *store.Store, disp *channel.Dispatcher, opts OutboxOptions) *Outbox {
	if st == nil || disp == nil {
		return nil
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = defaultOutboxWorkers
	}
	queue := opts.QueueSize
	if queue <= 0 {
		queue = defaultOutboxQueue
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultOutboxTimeout
	}
	o := &Outbox{
		store:      st,
		dispatcher: disp,
		jobs:       make(chan outboxJob, queue),
		timeout:    timeout,
	}
	o.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go o.worker()
	}
	log.Printf("notify outbox: %d workers, queue %d, timeout %s", workers, queue, timeout)
	return o
}

// Enqueue 投递入队；返回 false 表示队列满（已记日志与 dropped 计数）或已关闭。
func (o *Outbox) Enqueue(job outboxJob) bool {
	if o == nil || o.closed.Load() {
		return false
	}
	if !channel.Has(job.channels, "email") && !channel.Has(job.channels, "sms") && !channel.Has(job.channels, "voice") {
		return false
	}
	select {
	case o.jobs <- job:
		o.enqueued.Add(1)
		return true
	default:
		n := o.dropped.Add(1)
		log.Printf("notify outbox: queue full, dropped delivery (user=%d ref=%s/%s channels=%v, dropped_total=%d)",
			job.userID, job.refType, job.refID, job.channels, n)
		return false
	}
}

// Dropped 返回累计被丢弃的投递数（队列满）。
func (o *Outbox) Dropped() uint64 {
	if o == nil {
		return 0
	}
	return o.dropped.Load()
}

func (o *Outbox) worker() {
	defer o.wg.Done()
	for job := range o.jobs {
		o.deliver(job)
	}
}

func (o *Outbox) deliver(job outboxJob) {
	// 不挂在请求 context 上：请求早已返回，投递用独立超时。
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	results := o.dispatcher.DeliverExternal(ctx, job.channels, job.contact, job.payload)
	if len(results) == 0 {
		return
	}
	o.sent.Add(1)
	o.store.SaveDeliveryLogs(job.tenantID, job.userID, job.messageID,
		job.code, job.refType, job.refID, results)
}

// Shutdown 停止收新任务并等 in-flight 投递完成（或 ctx 超时）。
func (o *Outbox) Shutdown(ctx context.Context) error {
	if o == nil {
		return nil
	}
	o.closeOnce.Do(func() {
		o.closed.Store(true)
		close(o.jobs)
	})
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		log.Printf("notify outbox: shutdown timeout, %d queued deliveries not flushed", len(o.jobs))
		return ctx.Err()
	}
}

// deliverSyncLogs 同步投递并批量写日志（模板测试发送等运维单发路径仍走请求内，
// 控制台要当场看到 sent/failed 明细）。
func deliverSyncLogs(ctx context.Context, disp *channel.Dispatcher, st *store.Store,
	tenantID, userID, messageID uint64, code, refType, refID string,
	channels []string, contact channel.Contact, p channel.Payload) []channel.Result {
	results := disp.DeliverExternal(ctx, channels, contact, p)
	st.SaveDeliveryLogs(tenantID, userID, messageID, code, refType, refID, results)
	return results
}
