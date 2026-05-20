package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/captcha"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

// CaptchaAPI 验证码API
type CaptchaAPI struct{}

// NewCaptchaAPI 创建CaptchaAPI实例
func NewCaptchaAPI() *CaptchaAPI {
	return &CaptchaAPI{}
}

// GetCaptcha 获取验证码
func (a *CaptchaAPI) GetCaptcha(c *gin.Context) {
	key := captcha.GenerateCaptchaKey()
	data, err := captcha.GetTextCaptcha(key)
	if err != nil {
		response.InternalServerError(c, "生成验证码失败")
		return
	}

	response.Success(c, data)
}

// VerifyCaptcha 验证验证码
func (a *CaptchaAPI) VerifyCaptcha(c *gin.Context) {
	var req struct {
		Key  string `json:"key" binding:"required"`
		Code string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if !captcha.VerifyTextCaptcha(req.Key, req.Code) {
		response.BadRequest(c, "验证失败")
		return
	}

	response.SuccessWithMessage(c, "验证成功", nil)
}
