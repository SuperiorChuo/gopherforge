package config

import (
	"context"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// 呼叫中心语音服务运行时配置：TTS（提示音生成）与 ASR（录音转写）。
// 事实源是 system_settings 表（控制台「系统设置→呼叫中心」页，保存即热生效），
// CC_TTS_*/CC_ASR_* 环境变量只作兜底默认值——DB 非空字段逐项覆盖 env
// （对齐 ai-service runtimeconfig.AIProviderReader 的范式）。

const (
	// CCTTSSettingKey system_settings 行：TTS 文本转提示音（OpenAI 兼容 /v1/audio/speech）
	CCTTSSettingKey = "cc.tts"
	// CCASRSettingKey system_settings 行：ASR 录音转写（OpenAI 兼容 /v1/audio/transcriptions）
	CCASRSettingKey = "cc.asr"
)

// SpeechSettings TTS+ASR 生效值（env 兜底 + DB 覆盖后的结果）。
type SpeechSettings struct {
	TTSURL   string
	TTSKey   string
	TTSModel string
	TTSVoice string
	ASRURL   string
	ASRKey   string
	ASRModel string
}

// systemSettingRow 只映射需要的两列（表归 system-service 管，这里只读）。
type systemSettingRow struct {
	SettingKey string         `gorm:"column:setting_key;primaryKey"`
	ValueJSON  map[string]any `gorm:"column:value_json;type:json;serializer:json"`
}

func (systemSettingRow) TableName() string { return "system_settings" }

// SpeechReader 带 TTL 缓存的运行时配置读取器：30 秒内命中缓存，
// 过期重查 DB；DB 不可用/无记录时回落 env 默认值，绝不阻塞业务。
type SpeechReader struct {
	db  *gorm.DB
	env SpeechSettings
	ttl time.Duration

	mu        sync.RWMutex
	cached    SpeechSettings
	expiresAt time.Time
	loaded    bool
}

// NewSpeechReader env 为环境变量兜底值（来自 Load() 的 Cfg 字段）。
func NewSpeechReader(db *gorm.DB, env SpeechSettings) *SpeechReader {
	return &SpeechReader{db: db, env: env, ttl: 30 * time.Second}
}

// Settings 返回当前生效配置（缓存 30s；查库失败回落上次值或 env）。
func (r *SpeechReader) Settings(ctx context.Context) SpeechSettings {
	if r == nil {
		return SpeechSettings{}
	}
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		s := r.cached
		r.mu.RUnlock()
		return s
	}
	r.mu.RUnlock()

	s := r.env
	if r.db != nil {
		var rows []systemSettingRow
		err := r.db.WithContext(ctx).
			Where("setting_key IN ?", []string{CCTTSSettingKey, CCASRSettingKey}).
			Limit(2). // setting_key is the primary key; at most these two rows exist.
			Find(&rows).Error
		if err == nil {
			for _, row := range rows {
				switch row.SettingKey {
				case CCTTSSettingKey:
					s.TTSURL = strings.TrimRight(strSetting(row.ValueJSON, "url", s.TTSURL), "/")
					s.TTSKey = strSetting(row.ValueJSON, "api_key", s.TTSKey)
					s.TTSModel = strSetting(row.ValueJSON, "model", s.TTSModel)
					s.TTSVoice = strSetting(row.ValueJSON, "voice", s.TTSVoice)
				case CCASRSettingKey:
					s.ASRURL = strings.TrimRight(strSetting(row.ValueJSON, "url", s.ASRURL), "/")
					s.ASRKey = strSetting(row.ValueJSON, "api_key", s.ASRKey)
					s.ASRModel = strSetting(row.ValueJSON, "model", s.ASRModel)
				}
			}
		} else {
			// 查库失败：有旧缓存用旧缓存（哪怕过期），没有才落 env
			r.mu.RLock()
			if r.loaded {
				s = r.cached
			}
			r.mu.RUnlock()
			return s
		}
	}

	r.mu.Lock()
	r.cached = s
	r.expiresAt = now.Add(r.ttl)
	r.loaded = true
	r.mu.Unlock()
	return s
}

func strSetting(m map[string]any, key, fallback string) string {
	if m == nil {
		return fallback
	}
	if raw, ok := m[key].(string); ok {
		if v := strings.TrimSpace(raw); v != "" {
			return v
		}
	}
	return fallback
}
