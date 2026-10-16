package runner

import (
	"log"
	"time"
)

// Best effort: if storage is entirely unavailable the row can remain running;
// log the failed terminal write and let startup recovery handle it. Never turn
// a failed result/progress/completion write into a successful after-hook.
func (r *Runner) failGeoRun(tenantID, runID uint64, reason string) {
	log.Printf("geo run %d failed: %s", runID, reason)
	if err := r.store.UpdateGeoRun(tenantID, runID, map[string]any{"status": "failed", "error": taskError(reason), "finished_at": time.Now()}); err != nil {
		log.Printf("geo run %d persist failure state: %v", runID, err)
	}
}

func (r *Runner) failSerpTask(tenantID, taskID uint64, reason string) {
	log.Printf("serp task %d failed: %s", taskID, reason)
	if err := r.store.UpdateSerpTask(tenantID, taskID, map[string]any{"status": "failed", "error": taskError(reason), "finished_at": time.Now()}); err != nil {
		log.Printf("serp task %d persist failure state: %v", taskID, err)
	}
}

func taskError(text string) string {
	runes := []rune(text)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return text
}
