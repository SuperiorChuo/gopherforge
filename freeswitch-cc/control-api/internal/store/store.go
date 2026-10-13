package store

import (
	"errors"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/fullread"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Call struct {
	ID          uint64     `gorm:"primaryKey" json:"id"`
	CallID      string     `gorm:"size:64;uniqueIndex" json:"call_id"`
	Direction   string     `gorm:"size:16" json:"direction"`
	Caller      string     `gorm:"size:64" json:"caller"`
	Callee      string     `gorm:"size:64" json:"callee"`
	AgentExt    string     `gorm:"size:32" json:"agent_ext"`
	Queue       string     `gorm:"size:64" json:"queue"`
	Status      string     `gorm:"size:32" json:"status"`
	DurationSec int        `json:"duration_sec"`
	Recording   string     `gorm:"size:512" json:"recording"`
	RawEvent    string     `gorm:"type:text" json:"raw_event,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	AnsweredAt  *time.Time `json:"answered_at,omitempty"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Call) TableName() string { return "cc_calls" }

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := db.AutoMigrate(&Call{}, &Extension{}, &Gateway{}, &IVRMenu{}, &CCQueue{}, &CCQueueAgent{}, &BlacklistEntry{}, &CCConference{}, &CCTimeRoute{}, &CCRingGroup{}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) UpsertCall(c *Call) error {
	var existing Call
	err := s.db.Where("call_id = ?", c.CallID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(c).Error
	}
	if err != nil {
		return err
	}
	c.ID = existing.ID
	return s.db.Save(c).Error
}

func (s *Store) ListCalls(limit int) ([]Call, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []Call
	err := s.db.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

// Extension is a dynamically managed SIP extension. The DB row is the source
// of truth; the directory XML file is materialized from it (extdir 包).
type Extension struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	Ext      string `gorm:"size:16;uniqueIndex" json:"ext"`
	Password string `gorm:"size:64" json:"password"`
	Label    string `gorm:"size:64" json:"label"`
	// VMEnabled 开启语音信箱：无人接/占线转留言、可拨 *98 听取。
	// 禁挂 gorm default:true 标签——false 会被当零值省略，RETURNING 又把 DB
	// 默认值回写内存，导致物化 XML 与请求相反（对齐 Gateway.Register 的教训）。
	VMEnabled bool `json:"vm_enabled"`
	// VMPassword 语音信箱密码（4-8 位数字，*98 听留言时输）；空=跟随分机号。
	VMPassword string `gorm:"size:16" json:"vm_password"`
	// DirFirstName/DirLastName 拼音名（如 san/zhang）：mod_directory 411 按姓名
	// 拼音首字母查找同事时用；两者都空则该分机不进 411 目录（directory-visible）。
	DirFirstName string    `gorm:"size:32" json:"dir_first_name"`
	DirLastName  string    `gorm:"size:32" json:"dir_last_name"`
	CreatedAt    time.Time `json:"created_at"`
}

func (Extension) TableName() string { return "cc_extensions" }

func (s *Store) ListExtensions() ([]Extension, error) {
	return fullread.All[Extension](s.db.Order("ext ASC"))
}

func (s *Store) CreateExtension(e *Extension) error {
	return s.db.Create(e).Error
}

// GetExtension 按分机号查动态分机（mod_xml_curl directory 查询用）。
// 未命中返回 gorm.ErrRecordNotFound。
func (s *Store) GetExtension(ext string) (*Extension, error) {
	var e Extension
	if err := s.db.Where("ext = ?", ext).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// UpdateExtension 按分机号更新动态分机字段（密码/名称/语音信箱/411 目录）。
// 返回 gorm.ErrRecordNotFound 表示该分机不是动态分机（静态分机不可改）。
func (s *Store) UpdateExtension(ext string, e *Extension) error {
	res := s.db.Model(&Extension{}).Where("ext = ?", ext).Updates(map[string]any{
		"password":       e.Password,
		"label":          e.Label,
		"vm_enabled":     e.VMEnabled,
		"vm_password":    e.VMPassword,
		"dir_first_name": e.DirFirstName,
		"dir_last_name":  e.DirLastName,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteExtension returns gorm.ErrRecordNotFound when the ext is not dynamic.
func (s *Store) DeleteExtension(ext string) error {
	res := s.db.Where("ext = ?", ext).Delete(&Extension{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Gateway is a SIP trunk on the sofia external profile. DB 为事实源，XML 物化
// 到 sip_profiles/external 卷；DialPrefix 非空时同时物化出局拨号计划。
// 参数集对齐 sofia.conf 官方 gateway 示例（17 参数中业务相关的子集）。
type Gateway struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"size:32;uniqueIndex" json:"name"`
	Username string `gorm:"size:64" json:"username"`
	Password string `gorm:"size:64" json:"password"`
	Realm    string `gorm:"size:128" json:"realm"`
	Proxy    string `gorm:"size:128" json:"proxy"`
	// Register=false 仅作 IP 对接/免注册鉴权（不发 REGISTER）。
	// 注意不能挂 gorm default 标签：false 会被当零值省略，RETURNING 又把
	// DB 默认值回写内存，导致物化 XML 与请求相反。
	Register bool `json:"register"`
	// FromUser/FromDomain 出局 From 头（显号/域）；空则跟随 username/realm
	FromUser   string `gorm:"size:64" json:"from_user"`
	FromDomain string `gorm:"size:128" json:"from_domain"`
	// Extension 入方向落点：本线路来电路由到该号码（可填分机/队列/IVR 接入号）；
	// 空则跟随 username
	Extension string `gorm:"size:16" json:"extension"`
	// RegisterTransport udp/tcp/tls；空=udp
	RegisterTransport string `gorm:"size:8" json:"register_transport"`
	ExpireSeconds     int    `json:"expire_seconds"`
	RetrySeconds      int    `json:"retry_seconds"`
	// PingSeconds >0 时按间隔发 OPTIONS 保活探测，失败自动标记线路 DOWN
	PingSeconds    int  `json:"ping_seconds"`
	CallerIDInFrom bool `json:"caller_id_in_from"`
	// 出局号码变换：拨 <DialPrefix><号码> 命中本线路 → 去冠 → 加 Prepend（送号
	// 前缀/tech prefix）→ 送网关；CallerNumber 非空时改写出局主叫显号
	DialPrefix   string `gorm:"size:8" json:"dial_prefix"`
	Prepend      string `gorm:"size:16" json:"prepend"`
	CallerNumber string `gorm:"size:32" json:"caller_number"`
	// MaxConcurrent 本线路出局并发上限（mod_hash 后端 limit）：0=不限，>0 时
	// 出局拨号计划在 bridge 前插 `limit hash cc gw_<name> N !USER_BUSY`，超限
	// 直接 USER_BUSY。int 零值即"不限"，无需 gorm default 标签。
	MaxConcurrent int `json:"max_concurrent"`
	// LCR 费率选路：同一 DialPrefix 有多条网关时，按 RatePerMin 升序生成 failover
	// bridge（便宜线路先试，失败自动切下一条）。RatePerMin 单位为分/分钟，
	// 0=未设——排序时视为最贵（沉到组尾）。SortWeight 同费率时的平级手动排序，
	// 小的在前（默认 0）。
	RatePerMin int64 `json:"rate_per_min"`
	SortWeight int   `json:"sort_weight"`
	// TenantID 本线路归属的中台租户（转售计费）：>0 时出局拨号计划 export
	// cc_tenant_id=N，随 CDR/事件带回中台按租户入账；0=未挂租户（默认账）。
	// 媒体面自身不做租户隔离，仅透传标记。
	TenantID uint64 `json:"tenant_id"`
	// Disabled 停用（欠费冻结联动）：不物化 gateway XML（killgw 后注册断，
	// 入局失效）、不进出局拨号计划（字冠直拨失效）。DB 行保留，恢复即重物化。
	// 禁挂 gorm default 标签（bool 零值覆写教训，见 Register）。
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

func (Gateway) TableName() string { return "cc_gateways" }

func (s *Store) ListGateways() ([]Gateway, error) {
	return fullread.All[Gateway](s.db.Order("name ASC"))
}

func (s *Store) GetGateway(name string) (*Gateway, error) {
	var g Gateway
	if err := s.db.Where("name = ?", name).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) CreateGateway(g *Gateway) error { return s.db.Create(g).Error }

func (s *Store) UpdateGateway(g *Gateway) error { return s.db.Save(g).Error }

func (s *Store) DeleteGateway(name string) error {
	res := s.db.Where("name = ?", name).Delete(&Gateway{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SetGatewaysDisabledByTenant 批量停用/恢复某租户名下全部线路（欠费冻结联动）。
// 返回受影响的线路名（调用方据此 killgw 与重物化）。
func (s *Store) SetGatewaysDisabledByTenant(tenantID uint64, disabled bool) ([]string, error) {
	list, err := fullread.All[Gateway](s.db.Where("tenant_id = ?", tenantID))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(list))
	for _, g := range list {
		names = append(names, g.Name)
	}
	err = s.db.Model(&Gateway{}).Where("tenant_id = ?", tenantID).
		Update("disabled", disabled).Error
	return names, err
}

// CCQueue is a mod_callcenter queue. DB 为事实源，物化为 callcenter.conf 的
// include 片段；坐席-队列关系（tier）一并存这里。
type CCQueue struct {
	ID   uint64 `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:32;uniqueIndex" json:"name"`
	// AccessNumber 拨此号进队（物化接入号拨号计划）
	AccessNumber string `gorm:"size:16;uniqueIndex" json:"access_number"`
	// Strategy 派单策略：ring-all/longest-idle-agent/round-robin/top-down/
	// agent-with-least-talk-time/agent-with-fewest-calls/sequentially-by-agent-order/ring-progressively
	Strategy string `gorm:"size:40" json:"strategy"`
	// MOHSound 排队等待音乐：local_stream://moh 或上传文件路径
	MOHSound string `gorm:"size:256" json:"moh_sound"`
	// MaxWaitTime 超时秒数（0=不限），超时转 TimeoutAction 号码
	MaxWaitTime   int    `json:"max_wait_time"`
	TimeoutNumber string `gorm:"size:16" json:"timeout_number"`
	// TierRulesApply 逐层放开坐席（VIP 层优先）
	TierRulesApply  bool `json:"tier_rules_apply"`
	TierRuleWaitSec int  `json:"tier_rule_wait_sec"`
	// AnnounceSound 排队周期播报音（提示音库/MOH 文件路径），空=不播报
	AnnounceSound string `gorm:"size:256" json:"announce_sound"`
	// AnnounceFrequency 播报间隔秒（0=不播报）
	AnnounceFrequency int `json:"announce_frequency"`
	// RecordTemplate 队列级自动录音（进队即录），bool 禁 default 标签
	RecordTemplate bool `json:"record_template"`
	// AbandonedResumeAllowed 掉线回队保位（重连恢复排队位置），bool 禁 default 标签
	AbandonedResumeAllowed bool `json:"abandoned_resume_allowed"`
	// RingProgressivelyDelay ring-progressively 策略下逐个振铃的间隔秒
	RingProgressivelyDelay int `json:"ring_progressively_delay"`
	// SkipAgentsWithExternalCalls 跳过正在外呼的坐席，bool 禁 default 标签
	SkipAgentsWithExternalCalls bool `json:"skip_agents_with_external_calls"`
	// AgentNoAnswerStatus 坐席无应答后自动置为该状态：空/On Break/Logged Out
	AgentNoAnswerStatus string    `gorm:"size:16" json:"agent_no_answer_status"`
	CreatedAt           time.Time `json:"created_at"`
}

func (CCQueue) TableName() string { return "cc_cc_queues" }

// CCQueueAgent 把分机挂进队列（mod_callcenter agent+tier 的持久侧）。
type CCQueueAgent struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	QueueName string `gorm:"size:32;index;uniqueIndex:ux_ccq_agent" json:"queue_name"`
	Ext       string `gorm:"size:16;uniqueIndex:ux_ccq_agent" json:"ext"`
	// Level/Position tier 层级与层内顺序（小的先派）
	Level    int `json:"level"`
	Position int `json:"position"`
	// WrapUpTime 话后处理秒数，接完一通后歇这么久再派
	WrapUpTime  int `json:"wrap_up_time"`
	MaxNoAnswer int `json:"max_no_answer"`
	// RejectDelayTime/BusyDelayTime/NoAnswerDelayTime 拒接/示忙/无应答后
	// 该坐席的冷却秒数（0=用队列默认值）
	RejectDelayTime   int       `json:"reject_delay_time"`
	BusyDelayTime     int       `json:"busy_delay_time"`
	NoAnswerDelayTime int       `json:"no_answer_delay_time"`
	CreatedAt         time.Time `json:"created_at"`
}

func (CCQueueAgent) TableName() string { return "cc_queue_agents" }

func (s *Store) ListCCQueues() ([]CCQueue, error) {
	return fullread.All[CCQueue](s.db.Order("name ASC"))
}

func (s *Store) GetCCQueue(name string) (*CCQueue, error) {
	var q CCQueue
	if err := s.db.Where("name = ?", name).First(&q).Error; err != nil {
		return nil, err
	}
	return &q, nil
}

func (s *Store) CreateCCQueue(q *CCQueue) error { return s.db.Create(q).Error }
func (s *Store) UpdateCCQueue(q *CCQueue) error { return s.db.Save(q).Error }

func (s *Store) DeleteCCQueue(name string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("queue_name = ?", name).Delete(&CCQueueAgent{}).Error; err != nil {
			return err
		}
		res := tx.Where("name = ?", name).Delete(&CCQueue{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *Store) ListQueueAgents(queue string) ([]CCQueueAgent, error) {
	q := s.db.Order("queue_name ASC, level ASC, position ASC")
	if queue != "" {
		q = q.Where("queue_name = ?", queue)
	}
	return fullread.All[CCQueueAgent](q)
}

func (s *Store) UpsertQueueAgent(a *CCQueueAgent) error {
	var existing CCQueueAgent
	err := s.db.Where("queue_name = ? AND ext = ?", a.QueueName, a.Ext).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(a).Error
	}
	if err != nil {
		return err
	}
	a.ID = existing.ID
	a.CreatedAt = existing.CreatedAt
	return s.db.Save(a).Error
}

func (s *Store) DeleteQueueAgent(queue, ext string) error {
	res := s.db.Where("queue_name = ? AND ext = ?", queue, ext).Delete(&CCQueueAgent{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BlacklistEntry 骚扰号黑名单（mod_blacklist 文件 + 内存名单的持久侧）。
type BlacklistEntry struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Number    string    `gorm:"size:32;uniqueIndex" json:"number"`
	Remark    string    `gorm:"size:128" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}

func (BlacklistEntry) TableName() string { return "cc_blacklist" }

func (s *Store) ListBlacklist() ([]BlacklistEntry, error) {
	return fullread.All[BlacklistEntry](s.db.Order("id DESC"))
}

func (s *Store) CreateBlacklist(e *BlacklistEntry) error { return s.db.Create(e).Error }

func (s *Store) DeleteBlacklist(number string) error {
	res := s.db.Where("number = ?", number).Delete(&BlacklistEntry{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// IVRMenu is a keypad menu reachable at AccessNumber. Entries 存 JSON 数组
// [{digits, action, target}]；物化为 ivr_menus XML + 接入号拨号计划。
type IVRMenu struct {
	ID           uint64 `gorm:"primaryKey" json:"id"`
	Name         string `gorm:"size:32;uniqueIndex" json:"name"`
	AccessNumber string `gorm:"size:16;uniqueIndex" json:"access_number"`
	// Greeting 可放音文件路径或 tone_stream://...；空用默认提示音
	Greeting  string    `gorm:"size:256" json:"greeting"`
	TimeoutMS int       `gorm:"default:8000" json:"timeout_ms"`
	Entries   string    `gorm:"type:text" json:"entries"`
	CreatedAt time.Time `json:"created_at"`
}

func (IVRMenu) TableName() string { return "cc_ivr_menus" }

func (s *Store) ListIVRMenus() ([]IVRMenu, error) {
	return fullread.All[IVRMenu](s.db.Order("access_number ASC"))
}

func (s *Store) GetIVRMenu(name string) (*IVRMenu, error) {
	var m IVRMenu
	if err := s.db.Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) CreateIVRMenu(m *IVRMenu) error { return s.db.Create(m).Error }

func (s *Store) UpdateIVRMenu(m *IVRMenu) error { return s.db.Save(m).Error }

func (s *Store) DeleteIVRMenu(name string) error {
	res := s.db.Where("name = ?", name).Delete(&IVRMenu{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CCConference 是 mod_conference 的固定会议室（DB 为事实源，物化为拨号计划
// 40_conf-<name>.xml）。动态会议室（拨 39xx 随拨随建）不入库，只在物化时留
// 一个兜底拨号计划 40_conf-dynamic.xml。
type CCConference struct {
	ID uint64 `gorm:"primaryKey" json:"id"`
	// Name 会议室名（小写唯一），拨号计划里以 <Name>@default 命名 profile 实例
	Name string `gorm:"size:32;uniqueIndex" json:"name"`
	// Number 接入号（唯一，3xxx 且禁 39 开头——39xx 留给动态会议段）
	Number string `gorm:"size:16;uniqueIndex" json:"number"`
	// PIN 入会密码（可空，4-8 位数字）；空则拨号即入
	PIN string `gorm:"size:8" json:"pin"`
	// MaxMembers 人数上限（0=不限）
	MaxMembers int `json:"max_members"`
	// AutoRecord 进会自动录音（禁挂 default:true，false 会被当零值省略又被
	// RETURNING 回写，导致物化 XML 与请求相反）
	AutoRecord  bool      `json:"auto_record"`
	Description string    `gorm:"size:256" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

func (CCConference) TableName() string { return "cc_conferences" }

func (s *Store) ListConferences() ([]CCConference, error) {
	return fullread.All[CCConference](s.db.Order("number ASC"))
}

func (s *Store) GetConference(name string) (*CCConference, error) {
	var conf CCConference
	if err := s.db.Where("name = ?", name).First(&conf).Error; err != nil {
		return nil, err
	}
	return &conf, nil
}

func (s *Store) CreateConference(conf *CCConference) error { return s.db.Create(conf).Error }
func (s *Store) UpdateConference(conf *CCConference) error { return s.db.Save(conf).Error }

func (s *Store) DeleteConference(name string) error {
	res := s.db.Where("name = ?", name).Delete(&CCConference{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CCTimeRoute 营业时间路由：拨 AccessNumber 时按工作日/时段判断是否营业，
// 营业转 OpenTarget（队列/IVR/分机/响铃组接入号），非营业转 ClosedTarget
// （空则播放 ClosedPrompt 后挂断）。DB 为事实源，物化为拨号计划
// 15_time-<name>.xml（前缀 15 保证抢在队列/IVR 接入号之前匹配）。
type CCTimeRoute struct {
	ID   uint64 `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:32;uniqueIndex" json:"name"`
	// AccessNumber 拨此号进入时间路由（唯一，3-6 位数字）
	AccessNumber string `gorm:"size:16;uniqueIndex" json:"access_number"`
	// Wdays FS wday 语法（1=周日…7=周六），如 "2-6" 或 "2,4,6"，直接透传
	Wdays string `gorm:"size:32" json:"wdays"`
	// TimeRanges 逗号分隔多段，如 "09:00-12:00,13:30-18:00"；每段物化一条 open extension
	TimeRanges string `gorm:"size:128" json:"time_ranges"`
	// OpenTarget 营业中转的目标号码（队列/IVR/分机/响铃组接入号）
	OpenTarget string `gorm:"size:16" json:"open_target"`
	// ClosedTarget 非营业转（可空）；空=播放 ClosedPrompt 后挂断
	ClosedTarget string `gorm:"size:16" json:"closed_target"`
	// ClosedPrompt 非营业提示音文件路径（来自提示音库，可空）
	ClosedPrompt string    `gorm:"size:256" json:"closed_prompt"`
	CreatedAt    time.Time `json:"created_at"`
}

func (CCTimeRoute) TableName() string { return "cc_time_routes" }

func (s *Store) ListTimeRoutes() ([]CCTimeRoute, error) {
	return fullread.All[CCTimeRoute](s.db.Order("access_number ASC"))
}

func (s *Store) GetTimeRoute(name string) (*CCTimeRoute, error) {
	var t CCTimeRoute
	if err := s.db.Where("name = ?", name).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreateTimeRoute(t *CCTimeRoute) error { return s.db.Create(t).Error }
func (s *Store) UpdateTimeRoute(t *CCTimeRoute) error { return s.db.Save(t).Error }

func (s *Store) DeleteTimeRoute(name string) error {
	res := s.db.Where("name = ?", name).Delete(&CCTimeRoute{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CCRingGroup 响铃组：拨 AccessNumber 同振/顺振一组分机，全都没接转
// FailoverTarget。DB 为事实源，物化为拨号计划 45_rg-<name>.xml。
type CCRingGroup struct {
	ID   uint64 `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:32;uniqueIndex" json:"name"`
	// AccessNumber 拨此号触发响铃组（唯一，3-6 位数字）
	AccessNumber string `gorm:"size:16;uniqueIndex" json:"access_number"`
	// Strategy simultaneous 同振（逗号 bridge）/ sequential 顺振（竖线 bridge）
	Strategy string `gorm:"size:16" json:"strategy"`
	// Exts 逗号分隔分机列表，如 "1000,1001,1002"
	Exts string `gorm:"size:256" json:"exts"`
	// RingTimeout 振铃超时秒（默认 20）
	RingTimeout int `json:"ring_timeout"`
	// FailoverTarget 全都没接转（可空；空=挂断）
	FailoverTarget string    `gorm:"size:16" json:"failover_target"`
	CreatedAt      time.Time `json:"created_at"`
}

func (CCRingGroup) TableName() string { return "cc_ring_groups" }

func (s *Store) ListRingGroups() ([]CCRingGroup, error) {
	return fullread.All[CCRingGroup](s.db.Order("access_number ASC"))
}

func (s *Store) GetRingGroup(name string) (*CCRingGroup, error) {
	var g CCRingGroup
	if err := s.db.Where("name = ?", name).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) CreateRingGroup(g *CCRingGroup) error { return s.db.Create(g).Error }
func (s *Store) UpdateRingGroup(g *CCRingGroup) error { return s.db.Save(g).Error }

func (s *Store) DeleteRingGroup(name string) error {
	res := s.db.Where("name = ?", name).Delete(&CCRingGroup{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
