package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type staticReader struct{ settings Settings }

func (r staticReader) WeatherSettings(context.Context) Settings { return r.settings }

func newAmapStub(t *testing.T, ipCalls, weatherCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/ip", func(w http.ResponseWriter, r *http.Request) {
		ipCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "1", "adcode": "440300", "city": "深圳市",
		})
	})
	mux.HandleFunc("/v3/weather/weatherInfo", func(w http.ResponseWriter, r *http.Request) {
		weatherCalls.Add(1)
		// base → lives；all → forecasts（今日高低）
		if r.URL.Query().Get("extensions") == "all" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "1",
				"forecasts": []map[string]any{{
					"casts": []map[string]string{{
						"daytemp": "31", "nighttemp": "24",
					}},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "1",
			"lives": []map[string]string{{
				"province": "广东", "city": "深圳市", "adcode": "440300",
				"weather": "多云", "temperature": "28", "humidity": "70",
				"winddirection": "东南", "windpower": "≤3", "reporttime": "2026-07-16 16:00:00",
			}},
		})
	})
	return httptest.NewServer(mux)
}

func TestLiveByIPNotConfigured(t *testing.T) {
	svc := NewService(staticReader{})
	if _, err := svc.LiveByIP(context.Background(), "1.2.3.4"); err != ErrNotConfigured {
		t.Fatalf("LiveByIP() error = %v, want ErrNotConfigured", err)
	}
}

func TestLiveByIPPrivateAddressFallsBackToDefaultCity(t *testing.T) {
	var ipCalls, weatherCalls atomic.Int32
	server := newAmapStub(t, &ipCalls, &weatherCalls)
	defer server.Close()

	svc := NewServiceForTest(
		staticReader{Settings{AmapKey: "k", DefaultCity: "440300"}},
		server.Client(), server.URL,
	)
	live, err := svc.LiveByIP(context.Background(), "192.168.1.10")
	if err != nil {
		t.Fatalf("LiveByIP() error = %v", err)
	}
	if ipCalls.Load() != 0 {
		t.Fatalf("private IP must not hit the IP-location API, got %d calls", ipCalls.Load())
	}
	if live.City != "深圳市" || live.Weather != "多云" || live.Temperature != "28" {
		t.Fatalf("unexpected live payload: %+v", live)
	}
	if live.TempHigh != "31" || live.TempLow != "24" {
		t.Fatalf("temp range = %s/%s, want 31/24", live.TempHigh, live.TempLow)
	}
}

func TestLiveByIPPrivateAddressWithoutDefaultCityFails(t *testing.T) {
	svc := NewService(staticReader{Settings{AmapKey: "k"}})
	if _, err := svc.LiveByIP(context.Background(), "10.0.0.1"); err == nil {
		t.Fatal("LiveByIP() expected error when city cannot be resolved")
	}
}

func TestLiveByIPCachesWeatherPerCity(t *testing.T) {
	var ipCalls, weatherCalls atomic.Int32
	server := newAmapStub(t, &ipCalls, &weatherCalls)
	defer server.Close()

	svc := NewServiceForTest(
		staticReader{Settings{AmapKey: "k"}},
		server.Client(), server.URL,
	)
	for range 3 {
		if _, err := svc.LiveByIP(context.Background(), "113.87.1.1"); err != nil {
			t.Fatalf("LiveByIP() error = %v", err)
		}
	}
	// 首次 miss：实况 base + 预报 all 各 1 次；后续走内存缓存不再打上游
	if got := weatherCalls.Load(); got != 2 {
		t.Fatalf("weather API calls = %d, want 2 (base+all once, then cache)", got)
	}
	if got := ipCalls.Load(); got != 1 {
		t.Fatalf("ip API calls = %d, want 1 (ip cache hit expected)", got)
	}
}

func TestApplySettingLayersFields(t *testing.T) {
	got := ApplySetting(Settings{}, map[string]any{
		"amap_key": " k1 ", "default_city": "440300", "cache_minutes": float64(10),
	})
	if got.AmapKey != "k1" || got.DefaultCity != "440300" || got.CacheMinutes != 10 {
		t.Fatalf("ApplySetting() = %+v", got)
	}
	// 空值不覆盖
	got = ApplySetting(got, map[string]any{"amap_key": "", "cache_minutes": float64(0)})
	if got.AmapKey != "k1" || got.CacheMinutes != 10 {
		t.Fatalf("empty values must not override, got %+v", got)
	}
}
