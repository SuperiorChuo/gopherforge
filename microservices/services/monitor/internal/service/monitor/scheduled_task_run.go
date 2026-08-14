package monitor

import (
	"context"
	"fmt"
	"log"
	"time"

	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
)

type taskRunRecorder interface {
	StartTaskRunContext(ctx context.Context, run *localmodel.OpsTaskRun) error
	FinishTaskRunContext(ctx context.Context, runID, status, message, errorMessage string, finishedAt time.Time, durationMS int64) error
}

type scheduledTaskRun struct {
	recorder taskRunRecorder
	runID    string
	jobName  string
}

func beginScheduledTaskRun(ctx context.Context, dao jobDAO, job localmodel.ScheduledJob, trigger string, startedAt time.Time) *scheduledTaskRun {
	recorder, ok := dao.(taskRunRecorder)
	if !ok {
		return nil
	}
	description := job.Description
	if description == "" {
		description = job.Name
	}
	run := localmodel.OpsTaskRun{
		RunID: jobbeat.NewRunID(), TaskKey: fmt.Sprintf("monitor.scheduled.%d", job.ID),
		Service: "monitor-service", Description: truncateTaskRunText(description, 255),
		Source: "scheduler", TriggerType: trigger, Status: localmodel.TaskRunStatusRunning,
		Attempt: 1, StartedAt: startedAt, CreatedAt: startedAt,
	}
	if err := recorder.StartTaskRunContext(ctx, &run); err != nil {
		log.Printf("Failed to start task run for %s: %v", job.Name, err)
	}
	return &scheduledTaskRun{recorder: recorder, runID: run.RunID, jobName: job.Name}
}

func (run *scheduledTaskRun) Finish(ctx context.Context, runErr error, message string, finishedAt time.Time, durationMS int64) {
	if run == nil {
		return
	}
	status, output, errorMessage := localmodel.TaskRunStatusSucceeded, message, ""
	if runErr != nil {
		status, output, errorMessage = localmodel.TaskRunStatusFailed, "", runErr.Error()
	}
	if err := run.recorder.FinishTaskRunContext(ctx, run.runID, status,
		truncateTaskRunText(output, 2048), truncateTaskRunText(errorMessage, 2048),
		finishedAt, durationMS); err != nil {
		log.Printf("Failed to finish task run for %s: %v", run.jobName, err)
	}
}

func truncateTaskRunText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
