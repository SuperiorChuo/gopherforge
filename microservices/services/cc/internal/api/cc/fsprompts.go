package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os/exec"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/config"
)

// 提示音库代理 + TTS 预生成：
//   - 上传/列表/试听/删除完全透传 control-api /v1/prompts（复用 MOH 范式）。
//   - TTS：调 OpenAI 兼容 /v1/audio/speech 拿音频字节 → ffmpeg 转 8k 单声道 wav
//     → 以 multipart 传给 control-api /v1/prompts。管理面预生成，不动 FS 运行时。
//
// 提示音库是 FS 侧全局资源（无租户归属），全部端点仅平台管理员。

var fsPromptFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.(wav|mp3)$`)

// 生成名/音色的安全形态（TTS 落盘文件名 + OpenAI voice 参数）
var (
	ttsNameRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,120}$`)
	ttsVoiceRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)
)

// FSPromptsList handles GET /api/v1/cc/fs/prompts.
func (s *Server) FSPromptsList(c *gin.Context) { s.fsAdminProxyGet(c, "/v1/prompts") }

// FSPromptUpload handles POST /api/v1/cc/fs/prompts（透传 multipart）。
func (s *Server) FSPromptUpload(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	out, err := s.FS.PostMultipart(c.Request.Context(), "/v1/prompts", c.GetHeader("Content-Type"), c.Request.Body)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSPromptFile handles GET /api/v1/cc/fs/prompts/:file — 试听。
func (s *Server) FSPromptFile(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	file := c.Param("file")
	if !fsPromptFileRe.MatchString(file) {
		fail(c, http.StatusBadRequest, "invalid file name")
		return
	}
	resp, err := s.FS.Stream(c.Request.Context(), "/v1/prompts/"+file)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, map[string]string{
		"Content-Disposition": resp.Header.Get("Content-Disposition"),
	})
}

// FSPromptDelete handles DELETE /api/v1/cc/fs/prompts/:file.
func (s *Server) FSPromptDelete(c *gin.Context) {
	file := c.Param("file")
	if !fsPromptFileRe.MatchString(file) {
		fail(c, http.StatusBadRequest, "invalid file name")
		return
	}
	s.fsProxyDelete(c, "/v1/prompts/"+file)
}

type ttsReq struct {
	Name  string `json:"name" binding:"required"`
	Text  string `json:"text" binding:"required"`
	Voice string `json:"voice"`
}

// FSPromptTTS handles POST /api/v1/cc/fs/prompts/tts — 文本转语音预生成。
func (s *Server) FSPromptTTS(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	sp := s.Speech.Settings(c.Request.Context())
	if sp.TTSURL == "" {
		fail(c, http.StatusBadRequest, "TTS 未配置，请上传录音，或在「系统设置→呼叫中心」配置 TTS（.env CC_TTS_* 亦可兜底）*")
		return
	}
	var req ttsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "name and text required")
		return
	}
	if !ttsNameRe.MatchString(req.Name) {
		fail(c, http.StatusBadRequest, "name 只能含字母/数字/._-（≤120）")
		return
	}
	if len([]rune(req.Text)) > 4000 {
		fail(c, http.StatusBadRequest, "text 过长（≤4000 字）")
		return
	}
	voice := req.Voice
	if voice == "" {
		voice = sp.TTSVoice
	}
	if !ttsVoiceRe.MatchString(voice) {
		fail(c, http.StatusBadRequest, "invalid voice")
		return
	}

	// 1) OpenAI 兼容 /v1/audio/speech → 音频字节
	audio, err := s.synthesizeSpeech(c.Request.Context(), sp, req.Text, voice)
	if err != nil {
		fail(c, http.StatusBadGateway, "TTS 合成失败："+err.Error())
		return
	}
	// 2) ffmpeg 转 8k 单声道 wav（FS 电话音质）
	wav, err := transcodeTo8kWav(c.Request.Context(), audio)
	if err != nil {
		fail(c, http.StatusInternalServerError, "音频转码失败："+err.Error())
		return
	}
	// 3) multipart 传给 control-api /v1/prompts
	body, contentType, err := buildPromptMultipart(req.Name+".wav", wav)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	out, err := s.FS.PostMultipart(c.Request.Context(), "/v1/prompts", contentType, body)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// synthesizeSpeech POST {model,input,voice} 到 OpenAI 兼容端点，返回音频字节。
func (s *Server) synthesizeSpeech(ctx context.Context, sp config.SpeechSettings, text, voice string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]any{
		"model": sp.TTSModel,
		"input": text,
		"voice": voice,
	})
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sp.TTSURL+"/v1/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if sp.TTSKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+sp.TTSKey)
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("TTS provider status %d: %s", resp.StatusCode, truncateErr(data))
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("TTS provider returned empty audio")
	}
	return data, nil
}

// transcodeTo8kWav 用 ffmpeg 把任意输入音频转成 8kHz 单声道 wav（电话制式）。
func transcodeTo8kWav(ctx context.Context, in []byte) ([]byte, error) {
	ffCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ffCtx, "ffmpeg", "-i", "pipe:0", "-ar", "8000", "-ac", "1", "-f", "wav", "pipe:1")
	cmd.Stdin = bytes.NewReader(in)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, truncateErr(stderr.Bytes()))
	}
	if out.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty output")
	}
	return out.Bytes(), nil
}

// buildPromptMultipart 组装 field "file" 的 multipart 表单。
func buildPromptMultipart(filename string, data []byte) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

func truncateErr(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
