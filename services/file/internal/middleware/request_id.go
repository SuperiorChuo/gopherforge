package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const RequestIDKey = "request_id"

// RequestID attaches a stable request id to each request and response.
func RequestID(headerName string) gin.HandlerFunc {
	if headerName == "" {
		headerName = "X-Request-ID"
	}
	return func(c *gin.Context) {
		requestID := c.GetHeader(headerName)
		if requestID == "" {
			requestID = newRequestID()
		}

		c.Set(RequestIDKey, requestID)
		c.Header(headerName, requestID)
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	if value, ok := c.Get(RequestIDKey); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
