// Package callback 实例终态后的业务回写：同步 HTTP 回调业务方内部端点。
//
// 回调目标按 biz_type 经环境变量注册（BPM_CALLBACK_<BIZTYPE>=完整 URL，
// biz_type 小写化匹配，如 BPM_CALLBACK_ORDER → order），
// 引擎对 biz_type 保持不透明字符串，不携带任何业务类型。
//
// 失败重试：立即 + 1 分钟 + 5 分钟共三次；仍失败仅日志告警（审批事实
// 优先，不回滚终态），业务侧按 (biz_type,biz_id,instance_id) 幂等补偿。
package callback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Payload 终态回调体（契约见设计文档 §3.6；租户经 X-Tenant-ID 头传递）。
type Payload struct {
	InstanceID    uint64          `json:"instance_id"`
	DefinitionKey string          `json:"definition_key"`
	BizType       string          `json:"biz_type"`
	BizID         string          `json:"biz_id"`
	Result        string          `json:"result"` // approved|rejected|canceled
	FormSnapshot  json.RawMessage `json:"form_snapshot"`
	FinishedAt    string          `json:"finished_at"` // RFC3339
}

type Dispatcher struct {
	targets map[string]string
	token   string
	client  *http.Client
	// Delays 各次尝试前的等待（默认 立即/1min/5min）；测试可注入短间隔。
	Delays []time.Duration
}

func New(targets map[string]string, token string) *Dispatcher {
	return &Dispatcher{
		targets: targets,
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 10 * time.Second},
		Delays:  []time.Duration{0, time.Minute, 5 * time.Minute},
	}
}

// TargetsFromEnv 扫描 BPM_CALLBACK_<BIZTYPE>=url 形态的环境变量。
func TargetsFromEnv() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, found := strings.Cut(kv, "=")
		if !found || !strings.HasPrefix(k, "BPM_CALLBACK_") || k == "BPM_CALLBACK_TOKEN" {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		biz := strings.ToLower(strings.TrimPrefix(k, "BPM_CALLBACK_"))
		out[biz] = v
	}
	return out
}

// Targets 已注册回调的 biz_type 数（启动日志用）。
func (d *Dispatcher) Targets() int { return len(d.targets) }

// Dispatch 异步分发（事务提交后调用）；未注册回调的 biz_type 静默跳过。
func (d *Dispatcher) Dispatch(tenantID uint64, p Payload) {
	if d == nil || d.targets[p.BizType] == "" {
		return
	}
	go func() {
		if err := d.DispatchSync(tenantID, p); err != nil {
			log.Printf("bpm callback: 最终失败（三次重试后放弃，需人工补偿）instance=%d biz=%s/%s: %v",
				p.InstanceID, p.BizType, p.BizID, err)
		}
	}()
}

// DispatchSync 同步执行全部重试（测试直接调用；返回最终错误）。
func (d *Dispatcher) DispatchSync(tenantID uint64, p Payload) error {
	url := d.targets[p.BizType]
	if url == "" {
		return nil
	}
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	var lastErr error
	for i, delay := range d.Delays {
		time.Sleep(delay)
		if err := d.post(url, tenantID, body); err != nil {
			lastErr = err
			log.Printf("bpm callback: 第 %d/%d 次失败 instance=%d biz=%s/%s: %v",
				i+1, len(d.Delays), p.InstanceID, p.BizType, p.BizID, err)
			continue
		}
		if i > 0 {
			log.Printf("bpm callback: 第 %d 次重试成功 instance=%d biz=%s/%s",
				i+1, p.InstanceID, p.BizType, p.BizID)
		}
		return nil
	}
	return lastErr
}

func (d *Dispatcher) post(url string, tenantID uint64, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", strconv.FormatUint(tenantID, 10))
	if d.token != "" {
		req.Header.Set("X-Internal-Token", d.token)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
