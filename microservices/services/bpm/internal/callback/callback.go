// Package callback 实例终态后的业务回写：执行一次 HTTP 投递。
//
// 回调目标按 biz_type 经环境变量注册（BPM_CALLBACK_<BIZTYPE>=完整 URL，
// biz_type 小写化匹配，如 BPM_CALLBACK_DEMO_EXPENSE → demo_expense），
// 引擎对 biz_type 保持不透明字符串，不携带任何业务类型。
//
// 持久化、抢占和退避由 store/main 的 outbox worker 负责；业务侧仍须按
// (biz_type,biz_id,instance_id) 幂等处理。
package callback

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrTargetNotRegistered = errors.New("callback target not registered")

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
}

func New(targets map[string]string, token string) *Dispatcher {
	return &Dispatcher{
		targets: targets,
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 10 * time.Second},
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

// Deliver 执行一次投递。未注册目标返回 ErrTargetNotRegistered，由 worker
// 视为无需通知并删除任务，避免无效任务永久堆积。
func (d *Dispatcher) Deliver(tenantID uint64, p Payload) error {
	if d == nil {
		return ErrTargetNotRegistered
	}
	url := d.targets[p.BizType]
	if url == "" {
		return ErrTargetNotRegistered
	}
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return d.post(url, tenantID, body)
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
