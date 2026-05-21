package captcha

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/big"
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/pkg/cache"
)

const (
	textCaptchaWidth  = 120
	textCaptchaHeight = 42
	textCaptchaLength = 4
	textCaptchaChars  = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

// Deprecated: use GetTextCaptchaContext instead.
func GetTextCaptcha(key string) (any, error) {
	return GetTextCaptchaContext(context.Background(), key)
}

func GetTextCaptchaContext(ctx context.Context, key string) (any, error) {
	code, err := generateTextCaptchaCode()
	if err != nil {
		return nil, err
	}
	if err := cache.NewCacheService().SetLoginCaptchaContext(ctx, key, code); err != nil {
		return nil, err
	}
	imageBase64, err := renderTextCaptchaPNG(code)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"key":    key,
		"type":   "text",
		"image":  imageBase64,
		"width":  textCaptchaWidth,
		"height": textCaptchaHeight,
	}, nil
}

// Deprecated: use CheckTextCaptchaContext instead.
func CheckTextCaptcha(key, code string) bool {
	return CheckTextCaptchaContext(context.Background(), key, code)
}

func CheckTextCaptchaContext(ctx context.Context, key, code string) bool {
	return checkTextCaptchaContext(ctx, key, code, true)
}

// Deprecated: use VerifyTextCaptchaContext instead.
func VerifyTextCaptcha(key, code string) bool {
	return VerifyTextCaptchaContext(context.Background(), key, code)
}

func VerifyTextCaptchaContext(ctx context.Context, key, code string) bool {
	return checkTextCaptchaContext(ctx, key, code, false)
}

func checkTextCaptchaContext(ctx context.Context, key, code string, consume bool) bool {
	cacheService := cache.NewCacheService()
	stored, err := cacheService.GetLoginCaptchaContext(ctx, key)
	if err != nil {
		return false
	}
	if !textCaptchaMatches(stored, code) {
		return false
	}
	if consume {
		_ = cacheService.DelLoginCaptchaContext(ctx, key)
	}
	return true
}

func textCaptchaMatches(expected, submitted string) bool {
	return normalizeTextCaptchaCode(expected) != "" && normalizeTextCaptchaCode(expected) == normalizeTextCaptchaCode(submitted)
}

func normalizeTextCaptchaCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func generateTextCaptchaCode() (string, error) {
	var builder strings.Builder
	builder.Grow(textCaptchaLength)
	for i := 0; i < textCaptchaLength; i++ {
		index, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(textCaptchaChars))))
		if err != nil {
			return "", err
		}
		builder.WriteByte(textCaptchaChars[index.Int64()])
	}
	return builder.String(), nil
}

func renderTextCaptchaPNG(code string) (string, error) {
	img := image.NewRGBA(image.Rect(0, 0, textCaptchaWidth, textCaptchaHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{245, 248, 255, 255}}, image.Point{}, draw.Src)

	// Draw deterministic block glyphs. They are intentionally simple for the local console.
	x := 12
	for _, ch := range code {
		drawBlockGlyph(img, x, 8, byte(ch))
		x += 26
	}
	for x := 0; x < textCaptchaWidth; x += 9 {
		img.Set(x, (x*7)%textCaptchaHeight, color.RGBA{120, 150, 210, 120})
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func drawBlockGlyph(img *image.RGBA, left, top int, ch byte) {
	pattern := glyphPattern(ch)
	fg := color.RGBA{30, 70, 160, 255}
	for row, bits := range pattern {
		for col := 0; col < 5; col++ {
			if bits&(1<<(4-col)) == 0 {
				continue
			}
			rect := image.Rect(left+col*4, top+row*4, left+col*4+3, top+row*4+3)
			draw.Draw(img, rect, &image.Uniform{fg}, image.Point{}, draw.Src)
		}
	}
}

func glyphPattern(ch byte) []byte {
	patterns := map[byte][]byte{
		'2': {0x1E, 0x01, 0x01, 0x1E, 0x10, 0x10, 0x1F},
		'3': {0x1E, 0x01, 0x01, 0x0E, 0x01, 0x01, 0x1E},
		'4': {0x12, 0x12, 0x12, 0x1F, 0x02, 0x02, 0x02},
		'5': {0x1F, 0x10, 0x10, 0x1E, 0x01, 0x01, 0x1E},
		'6': {0x0F, 0x10, 0x10, 0x1E, 0x11, 0x11, 0x0E},
		'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
		'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
		'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x01, 0x1E},
		'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
		'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
		'C': {0x0F, 0x10, 0x10, 0x10, 0x10, 0x10, 0x0F},
		'D': {0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E},
		'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
		'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
		'G': {0x0F, 0x10, 0x10, 0x13, 0x11, 0x11, 0x0F},
		'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
		'J': {0x01, 0x01, 0x01, 0x01, 0x11, 0x11, 0x0E},
		'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
		'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
		'M': {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
		'N': {0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11},
		'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
		'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
		'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
		'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
		'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
		'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
		'V': {0x11, 0x11, 0x11, 0x11, 0x0A, 0x0A, 0x04},
		'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x15, 0x0A},
		'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
		'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
		'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	}
	if pattern, ok := patterns[ch]; ok {
		return pattern
	}
	return patterns['8']
}

func GenerateCaptchaKey() string {
	return fmt.Sprintf("captcha:%d", time.Now().UnixNano())
}
