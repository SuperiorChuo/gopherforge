package system

import (
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResolveNoticeCreatorDefaultsToEnglishSystemActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	creatorID, creatorName := resolveNoticeCreator(ctx)
	if creatorID != 0 {
		t.Fatalf("creatorID = %d, want 0", creatorID)
	}
	if creatorName != "system" {
		t.Fatalf("creatorName = %q, want %q", creatorName, "system")
	}
}

func TestResolveNoticeCreatorUsesAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("user_id", uint(7))
	ctx.Set("username", "alice")

	creatorID, creatorName := resolveNoticeCreator(ctx)
	if creatorID != 7 {
		t.Fatalf("creatorID = %d, want 7", creatorID)
	}
	if creatorName != "alice" {
		t.Fatalf("creatorName = %q, want %q", creatorName, "alice")
	}
}

func TestNoticeAPIMessagesUseEnglish(t *testing.T) {
	content, err := os.ReadFile("notice.go")
	if err != nil {
		t.Fatalf("read notice.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("notice.go contains non-English source text")
	}
}
