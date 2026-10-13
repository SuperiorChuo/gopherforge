package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/config"
)

const (
	earlyMediaMaxBytes = 2 << 20
	earlyMediaTimeout  = 15 * time.Second
)

// earlyMediaPhrases 只收录运营商固定提示语，避免普通对话单个词误杀。
var earlyMediaPhrases = []struct {
	class   string
	phrases []string
}{
	{"empty", []string{"您拨打的号码是空号", "您拨打的号码不存在", "号码不存在", "号码有误", "查无此号", "尚未开通"}},
	{"unavailable", []string{"已关机", "暂时无法接通", "不在服务区", "已停机", "暂停服务", "无法为您接通"}},
	{"busy", []string{"正在通话中", "用户正忙", "线路正忙", "请稍后再拨"}},
}

func validEarlyMediaClass(class string) bool {
	return class == "empty" || class == "unavailable" || class == "busy"
}

func classifyEarlyMedia(text string) string {
	normalized := strings.NewReplacer(" ", "", "，", "", ",", "", "。", "", ".", "").Replace(strings.ToLower(text))
	for _, group := range earlyMediaPhrases {
		for _, phrase := range group.phrases {
			if strings.Contains(normalized, phrase) {
				return group.class
			}
		}
	}
	return ""
}

// InternalEarlyMediaClassify 接收 control-api 的短早媒体 WAV，调用现有
// OpenAI-compatible ASR 热配置。只返回类别，不保存/回传转写正文。
func (s *Server) InternalEarlyMediaClassify(c *gin.Context) {
	if s.ProductionMode && s.WebhookSecret == "" {
		fail(c, http.StatusServiceUnavailable, "early-media disabled: CC_WEBHOOK_SECRET is not configured")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, earlyMediaMaxBytes+1))
	if err != nil {
		fail(c, http.StatusBadRequest, "read audio failed")
		return
	}
	if len(raw) > earlyMediaMaxBytes {
		fail(c, http.StatusRequestEntityTooLarge, "audio too large")
		return
	}
	if len(raw) < 44 || !bytes.Equal(raw[:4], []byte("RIFF")) {
		fail(c, http.StatusBadRequest, "invalid wav")
		return
	}
	if !validEarlyMediaSignature(raw, c.GetHeader("X-CC-Signature"), s.WebhookSecret) {
		fail(c, http.StatusUnauthorized, "invalid signature")
		return
	}
	sp := s.Speech.Settings(c.Request.Context())
	if sp.ASRURL == "" {
		fail(c, http.StatusServiceUnavailable, "ASR 未配置")
		return
	}
	text, err := transcribeEarlyMedia(c.Request.Context(), sp, raw)
	if err != nil {
		fail(c, http.StatusBadGateway, "ASR 识别失败")
		return
	}
	class := classifyEarlyMedia(text)
	ok(c, gin.H{"blocked": class != "", "class": class})
}

func validEarlyMediaSignature(raw []byte, header, secret string) bool {
	if secret == "" {
		return true
	}
	sig := strings.TrimPrefix(header, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	want := hex.EncodeToString(mac.Sum(nil))
	return sig != "" && hmac.Equal([]byte(sig), []byte(want))
}

func transcribeEarlyMedia(ctx context.Context, sp config.SpeechSettings, wav []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, earlyMediaTimeout)
	defer cancel()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "early-media.wav")
	if err != nil {
		return "", err
	}
	if _, err = part.Write(wav); err != nil {
		return "", err
	}
	if err = mw.WriteField("model", sp.ASRModel); err != nil {
		return "", err
	}
	if err = mw.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	if err = mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sp.ASRURL+"/v1/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if sp.ASRKey != "" {
		req.Header.Set("Authorization", "Bearer "+sp.ASRKey)
	}
	resp, err := (&http.Client{Timeout: earlyMediaTimeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	text, _, err := parseASRResponse(resp)
	return text, err
}
