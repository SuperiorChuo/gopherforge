package monitor

import (
	"os"
	"regexp"
	"testing"
)

func TestMonitorServiceUsesEnglishSourceText(t *testing.T) {
	for _, file := range []string{
		"alert.go",
		"alert_metric.go",
		"alert_notification.go",
		"alert_scheduler.go",
		"job.go",
		"mysql.go",
		"redis.go",
		"server.go",
	} {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}

			if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
				t.Fatalf("%s contains non-English source text", file)
			}
		})
	}
}
