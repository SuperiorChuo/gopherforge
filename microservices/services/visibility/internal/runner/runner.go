// Package runner executes GEO runs and SERP check tasks asynchronously.
// 单实例设计：内存 map 做同租户并发去重，DB status 双保险；
// 多实例部署前需要改成分布式锁（Redis SetNX 等）。
package runner

import (
	"context"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"github.com/go-admin-kit/services/visibility/internal/engine"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/serp"
	"github.com/go-admin-kit/services/visibility/internal/store/visibility"
	"log"
	"sync"
	"time"
)

// Store 是 runner 依赖的窄接口，便于单测注入 fake。
type Store interface {
	ListActiveGeoPrompts(tenantID uint64) ([]model.GeoPrompt, error)
	ListAllBrands(tenantID uint64) ([]model.Brand, error)
	CreateGeoRun(*model.GeoRun) error
	UpdateGeoRun(tenantID, id uint64, updates map[string]any) error
	CreateGeoResult(*model.GeoResult) error
	FindRunningGeoRun(tenantID uint64) (*model.GeoRun, error)
	GetGeoRun(tenantID, id uint64) (*model.GeoRun, error)
	ListGeoResults(tenantID, runID uint64) ([]model.GeoResult, error)

	ListTrackingKeywords(tenantID, keywordID uint64) ([]model.Keyword, error)
	CreateSerpTask(*model.SerpTask) error
	UpdateSerpTask(tenantID, id uint64, updates map[string]any) error
	FindRunningSerpTask(tenantID uint64) (*model.SerpTask, error)
	CreateRankSnapshot(*model.RankSnapshot) error
	ApplyEngineRanks(tenantID, keywordID uint64, ranks []store.EngineRank) error

	GetSettingJSON(key string) (map[string]any, error)
	FindPreviousDoneGeoRun(tenantID, beforeID uint64) (*model.GeoRun, error)
	ListAdminUserIDs(ctx context.Context, tenantID uint64) ([]uint64, error)
}

// AIConfig 是 GEO 跑题用的模型配置（中转站，OpenAI 兼容）。
type AIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type Runner struct {
	store      Store
	ai         AIConfig
	collectors []serp.Collector
	maxPages   int
	// perplexity / gemini 真实引擎配置（可选，env 注入）。未配 key 时对应引擎不可用。
	perplexity AIConfig
	gemini     AIConfig
	// notify 为 nil 或未启用时静默跳过（脚手架无 notify-service 的场景）
	notify *notifyclient.Client

	// fetchText 抓取单页正文（默认 engine.FetchSiteText），测试可注入 fake 避免联网。
	fetchText func(ctx context.Context, url string, maxRunes int) (string, error)

	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	lifecycleMu  sync.Mutex
	closed       bool
	shutdownOnce sync.Once
	shutdownDone chan struct{}

	mu         sync.Mutex
	geoActive  map[uint64]uint64 // tenantID -> runID
	serpActive map[uint64]uint64 // tenantID -> taskID
}

func New(st Store, ai AIConfig, collectors []serp.Collector, maxPages int, notify *notifyclient.Client) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	if maxPages <= 0 {
		maxPages = 5
	}
	return &Runner{
		store:        st,
		ai:           ai,
		notify:       notify,
		collectors:   collectors,
		maxPages:     maxPages,
		fetchText:    engine.FetchSiteText,
		ctx:          ctx,
		cancel:       cancel,
		shutdownDone: make(chan struct{}),
		geoActive:    map[uint64]uint64{},
		serpActive:   map[uint64]uint64{},
	}
}

// Shutdown 取消所有任务 goroutine 并最多等 5 秒；被打断的任务会尽力标记 failed，
// 没来得及标记的由下次启动的 FailStaleTasks 兜底。
func (r *Runner) Shutdown() {
	// 先在生命周期锁内关闭注册入口，再开始 Wait。registerTask 与这里互斥，
	// 因而 Wait 开始后不会再把计数从 0 加到 1；重复 Shutdown 也安全。
	r.lifecycleMu.Lock()
	if !r.closed {
		r.closed = true
		r.cancel()
	}
	r.lifecycleMu.Unlock()
	r.shutdownOnce.Do(func() {
		go func() {
			r.wg.Wait()
			close(r.shutdownDone)
		}()
	})
	select {
	case <-r.shutdownDone:
	case <-time.After(5 * time.Second):
		log.Printf("runner: shutdown wait timed out")
	}
}

// registerTask 在 Shutdown 的等待边界前原子注册一项异步或同步后台任务。
func (r *Runner) registerTask() bool {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.closed {
		return false
	}
	r.wg.Add(1)
	return true
}
