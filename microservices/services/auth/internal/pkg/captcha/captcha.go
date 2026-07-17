package captcha

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/go-admin-kit/services/auth/internal/pkg/cache"
)

const (
	textCaptchaWidth  = 120
	textCaptchaHeight = 42
	textCaptchaLength = 4
	textCaptchaChars  = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

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

func CheckTextCaptchaContext(ctx context.Context, key, code string) bool {
	return checkTextCaptchaContext(ctx, key, code, true)
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

	// 柔和白蓝玻璃底（与登录页液态玻璃配色一致）
	for y := 0; y < textCaptchaHeight; y++ {
		for x := 0; x < textCaptchaWidth; x++ {
			// 横向微渐变 + 竖直提亮
			t := float64(x) / float64(textCaptchaWidth-1)
			v := float64(y) / float64(textCaptchaHeight-1)
			r := uint8(232 + 12*(1-t) + 8*(1-v))
			g := uint8(236 + 10*(1-t) + 6*(1-v))
			b := uint8(250 - 6*t + 4*(1-v))
			img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
		}
	}

	// 极淡干扰弧（非硬噪点）
	drawSoftArc(img, 18, 22, 48, color.RGBA{129, 140, 248, 55})
	drawSoftArc(img, 70, 28, 36, color.RGBA{167, 139, 250, 45})
	// 稀疏微尘
	for i := 0; i < 28; i++ {
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(textCaptchaWidth*textCaptchaHeight)))
		if err != nil {
			break
		}
		px := int(n.Int64()) % textCaptchaWidth
		py := int(n.Int64()) / textCaptchaWidth
		if py >= textCaptchaHeight {
			continue
		}
		img.SetRGBA(px, py, color.RGBA{99, 102, 241, 40})
	}

	// 圆角方块字形 + 轻微阴影
	x := 10
	for i, ch := range code {
		// 字符色在 indigo / violet 间轻变
		fg := color.RGBA{55, 70, 180, 255}
		if i%2 == 1 {
			fg = color.RGBA{88, 60, 190, 255}
		}
		drawBlockGlyph(img, x, 7, byte(ch), fg)
		x += 27
	}

	// 顶缘高光一条（玻璃感）
	for x := 4; x < textCaptchaWidth-4; x++ {
		boost := 40
		if x < 12 || x > textCaptchaWidth-12 {
			boost = 18
		}
		c := img.RGBAAt(x, 1)
		img.SetRGBA(x, 1, color.RGBA{
			min255(int(c.R) + boost),
			min255(int(c.G) + boost),
			min255(int(c.B) + boost),
			255,
		})
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func min255(v int) uint8 {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return uint8(v)
}

func drawSoftArc(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	for deg := 20; deg < 160; deg += 2 {
		rad := float64(deg) * math.Pi / 180
		x := cx + int(float64(radius)*math.Cos(rad))
		y := cy + int(float64(radius)*0.45*math.Sin(rad))
		if x < 0 || y < 0 || x >= textCaptchaWidth || y >= textCaptchaHeight {
			continue
		}
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				xx, yy := x+dx, y+dy
				if xx < textCaptchaWidth && yy < textCaptchaHeight {
					blendPixel(img, xx, yy, c)
				}
			}
		}
	}
}

func blendPixel(img *image.RGBA, x, y int, overlay color.RGBA) {
	base := img.RGBAAt(x, y)
	oa := float64(overlay.A) / 255
	inv := 1 - oa
	img.SetRGBA(x, y, color.RGBA{
		uint8(float64(base.R)*inv + float64(overlay.R)*oa),
		uint8(float64(base.G)*inv + float64(overlay.G)*oa),
		uint8(float64(base.B)*inv + float64(overlay.B)*oa),
		255,
	})
}

func drawBlockGlyph(img *image.RGBA, left, top int, ch byte, fg color.RGBA) {
	pattern := glyphPattern(ch)
	// 软阴影
	shadow := color.RGBA{99, 102, 241, 50}
	for row, bits := range pattern {
		for col := 0; col < 5; col++ {
			if bits&(1<<(4-col)) == 0 {
				continue
			}
			// shadow offset
			for sy := 0; sy < 3; sy++ {
				for sx := 0; sx < 3; sx++ {
					xx := left + col*4 + sx + 1
					yy := top + row*4 + sy + 1
					if xx >= 0 && yy >= 0 && xx < textCaptchaWidth && yy < textCaptchaHeight {
						blendPixel(img, xx, yy, shadow)
					}
				}
			}
			// 主色 3x3 块（圆角感：四角略淡）
			for sy := 0; sy < 3; sy++ {
				for sx := 0; sx < 3; sx++ {
					xx := left + col*4 + sx
					yy := top + row*4 + sy
					if xx < 0 || yy < 0 || xx >= textCaptchaWidth || yy >= textCaptchaHeight {
						continue
					}
					c := fg
					// 四角降不透明，略抗锯齿
					if (sx == 0 || sx == 2) && (sy == 0 || sy == 2) {
						c.A = 200
						blendPixel(img, xx, yy, c)
					} else {
						img.SetRGBA(xx, yy, c)
					}
				}
			}
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
