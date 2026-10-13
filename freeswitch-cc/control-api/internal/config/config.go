package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	APIToken      string
	ESLHost       string
	ESLPort       string
	ESLPassword   string
	DBDSN         string
	WebhookURL    string
	WebhookSecret string
	// RiskURL 中台风控检查（软电话出局）。空=不拦截。
	RiskURL string
	// EarlyMediaURL 中台早媒体短音频分类端点。空=关闭。
	EarlyMediaURL string
	// EarlyMediaDir 与 FreeSWITCH 隔离共享卷在 control-api 内的路径。
	EarlyMediaDir string
	// EarlyMediaSeconds 采样秒数，范围由调用侧钳制到 2-8 秒。
	EarlyMediaSeconds int
	// ExtDir 动态分机 XML 落盘目录（与 FS 容器共享卷 → /etc/freeswitch/directory/default）
	ExtDir string
	// RecDir 录音目录（与 FS 容器共享卷，只读）
	RecDir string
	// GwDir SIP 网关 XML（→ /etc/freeswitch/sip_profiles/external）
	GwDir string
	// IVRDir IVR 菜单 XML（→ /etc/freeswitch/ivr_menus）
	IVRDir string
	// DialplanDir 动态拨号计划（→ /etc/freeswitch/dialplan/default）
	DialplanDir string
	// CCQueueDir mod_callcenter 队列配置（→ callcenter.conf include 卷）
	CCQueueDir string
	// MOHDir 排队音乐上传目录（local_stream://moh 扫描 + moh-sound 直指）
	MOHDir string
	// PromptsDir 提示音库目录（普通文件即取即播，队列 announce-sound/IVR 可直指）
	PromptsDir string
	// BlacklistDir mod_blacklist 名单文件目录
	BlacklistDir string
	// VMDir 语音信箱留言存储目录（与 FS 共享卷只读，扫 default/<domain>/<ext>/msg_*.wav）
	VMDir string
	// Domain FS 全局 SIP 域（留言目录 default/<domain>/<ext> 与 vm_read 命令用）
	Domain string
}

func Load() Config {
	return Config{
		Port:              getenv("CC_API_PORT", "8090"),
		APIToken:          getenv("CC_API_TOKEN", "dev-cc-token-change-me"),
		ESLHost:           getenv("CC_ESL_HOST", "127.0.0.1"),
		ESLPort:           getenv("CC_ESL_PORT", "8021"),
		ESLPassword:       getenv("CC_ESL_PASSWORD", "ClueCon"),
		DBDSN:             getenv("CC_DB_DSN", "postgres://fscc:fscc123@127.0.0.1:5434/fs_cc?sslmode=disable"),
		WebhookURL:        getenv("CC_WEBHOOK_URL", ""),
		WebhookSecret:     getenv("CC_WEBHOOK_SECRET", "dev-webhook-secret-change-me"),
		RiskURL:           getenv("CC_RISK_URL", ""),
		EarlyMediaURL:     getenv("CC_EARLY_MEDIA_URL", ""),
		EarlyMediaDir:     getenv("CC_EARLY_MEDIA_DIR", "/data/early-media"),
		EarlyMediaSeconds: getenvInt("CC_EARLY_MEDIA_SECONDS", 4),
		ExtDir:            getenv("CC_EXT_DIR", "/data/extensions"),
		RecDir:            getenv("CC_RECORDINGS_DIR", "/data/recordings"),
		GwDir:             getenv("CC_GW_DIR", "/data/gateways"),
		IVRDir:            getenv("CC_IVR_DIR", "/data/ivr_menus"),
		DialplanDir:       getenv("CC_DIALPLAN_DIR", "/data/dialplan"),
		CCQueueDir:        getenv("CC_QUEUE_DIR", "/data/cc_queues"),
		MOHDir:            getenv("CC_MOH_DIR", "/data/moh"),
		PromptsDir:        getenv("CC_PROMPTS_DIR", "/data/prompts"),
		BlacklistDir:      getenv("CC_BLACKLIST_DIR", "/data/blacklist"),
		VMDir:             getenv("CC_VM_DIR", "/data/voicemail"),
		Domain:            getenv("CC_DOMAIN", ""),
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getenvInt(k string, d int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return d
}
