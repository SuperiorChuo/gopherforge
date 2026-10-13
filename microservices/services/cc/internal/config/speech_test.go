package config

import (
	"context"
	"github.com/go-admin-kit/services/internal/testdb"
	"testing"
	"time"
)

func TestSpeechSettingsKeyBoundAndFallback(t *testing.T) {
	db := testdb.Open(t, &systemSettingRow{})
	rows := []systemSettingRow{{SettingKey: CCTTSSettingKey, ValueJSON: map[string]any{"url": "http://tts.fixture/", "model": "db-tts", "voice": "fixture-voice"}}, {SettingKey: CCASRSettingKey, ValueJSON: map[string]any{"url": "http://asr.fixture/", "model": "db-asr"}}, {SettingKey: "unrelated", ValueJSON: map[string]any{"model": "wrong"}}}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	env := SpeechSettings{TTSKey: "fixture-tts-fallback", ASRKey: "fixture-asr-fallback", TTSModel: "env-tts", ASRModel: "env-asr"}
	r := NewSpeechReader(db, env)
	ctx := context.Background()
	got := r.Settings(ctx)
	if got.TTSURL != "http://tts.fixture" || got.ASRURL != "http://asr.fixture" || got.TTSModel != "db-tts" || got.ASRModel != "db-asr" || got.TTSVoice != "fixture-voice" || got.TTSKey != env.TTSKey || got.ASRKey != env.ASRKey {
		t.Fatalf("settings=%+v", got)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	r.expiresAt = time.Time{}
	if fallback := r.Settings(canceled); fallback != got {
		t.Fatalf("lost cached fallback: %+v", fallback)
	}
	if fallback := NewSpeechReader(db, env).Settings(canceled); fallback != env {
		t.Fatalf("lost env fallback: %+v", fallback)
	}
	if err := db.Where("setting_key IN ?", []string{CCTTSSettingKey, CCASRSettingKey}).Delete(&systemSettingRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if empty := NewSpeechReader(db, env).Settings(ctx); empty != env {
		t.Fatalf("missing keys fallback=%+v", empty)
	}
}
