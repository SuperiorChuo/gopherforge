package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/captcha"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

// CaptchaAPI handles captcha endpoints.
type CaptchaAPI struct{}

// NewCaptchaAPI creates a CaptchaAPI instance.
func NewCaptchaAPI() *CaptchaAPI {
	return &CaptchaAPI{}
}

// GetCaptcha returns a text captcha image payload.
func (a *CaptchaAPI) GetCaptcha(c *gin.Context) {
	key := captcha.GenerateCaptchaKey()
	data, err := captcha.GetTextCaptchaContext(c.Request.Context(), key)
	if err != nil {
		response.InternalServerError(c, "failed to generate captcha")
		return
	}

	response.Success(c, data)
}

// VerifyCaptcha validates a captcha code.
func (a *CaptchaAPI) VerifyCaptcha(c *gin.Context) {
	var req struct {
		Key  string `json:"key" binding:"required"`
		Code string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	if !captcha.VerifyTextCaptchaContext(c.Request.Context(), req.Key, req.Code) {
		response.BadRequest(c, "captcha verification failed")
		return
	}

	response.SuccessWithMessage(c, "captcha verified", nil)
}
