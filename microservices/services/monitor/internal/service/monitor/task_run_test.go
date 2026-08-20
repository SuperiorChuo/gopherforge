package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	monitordao "github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
)

type fakeTaskRunReader struct {
	list       []localmodel.OpsTaskRun
	total      int64
	summary    monitordao.TaskRunSummaryRow
	lastPage   pagination.PageRequest
	lastFilter monitordao.TaskRunFilter
}

func (f *fakeTaskRunReader) ListTaskRunsContext(_ context.Context, req pagination.PageRequest, filter monitordao.TaskRunFilter) ([]localmodel.OpsTaskRun, int64, error) {
	f.lastPage, f.lastFilter = req, filter
	return f.list, f.total, nil
}
func (f *fakeTaskRunReader) GetTaskRunByIDContext(_ context.Context, id uint64) (*localmodel.OpsTaskRun, error) {
	for i := range f.list {
		if f.list[i].ID == id {
			return &f.list[i], nil
		}
	}
	return nil, errors.New("not found")
}
func (f *fakeTaskRunReader) GetTaskRunSummaryContext(_ context.Context, _ time.Time) (monitordao.TaskRunSummaryRow, error) {
	return f.summary, nil
}

func TestTaskRunListValidatesAndCapsPagination(t *testing.T) {
	dao := &fakeTaskRunReader{total: 1}
	service := newTaskRunService(dao)
	_, _, err := service.ListContext(context.Background(), TaskRunQuery{
		PageRequest: pagination.PageRequest{Page: 2, PageSize: 999},
		Keyword:     "health", Service: "monitor-service", Status: localmodel.TaskRunStatusSucceeded, Source: "scheduler",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dao.lastPage.Page != 2 || dao.lastPage.PageSize != MaxTaskRunPageSize {
		t.Fatalf("page = %#v", dao.lastPage)
	}
	if dao.lastFilter.Keyword != "health" || dao.lastFilter.Service != "monitor-service" {
		t.Fatalf("filter = %#v", dao.lastFilter)
	}

	_, _, err = service.ListContext(context.Background(), TaskRunQuery{Status: "unknown"})
	if !errors.Is(err, ErrInvalidTaskRunStatus) {
		t.Fatalf("status error = %v", err)
	}
	_, _, err = service.ListContext(context.Background(), TaskRunQuery{Source: "business-payload"})
	if !errors.Is(err, ErrInvalidTaskRunSource) {
		t.Fatalf("source error = %v", err)
	}
}

func TestTaskRunListRejectsReverseTimeRange(t *testing.T) {
	service := newTaskRunService(&fakeTaskRunReader{})
	start, end := time.Now(), time.Now().Add(-time.Hour)
	_, _, err := service.ListContext(context.Background(), TaskRunQuery{StartAt: &start, EndAt: &end})
	if !errors.Is(err, ErrInvalidTaskRunRange) {
		t.Fatalf("range error = %v", err)
	}
}

func TestTaskRunSummaryCalculatesSuccessRate(t *testing.T) {
	service := newTaskRunService(&fakeTaskRunReader{summary: monitordao.TaskRunSummaryRow{
		Total: 7, Running: 1, Succeeded: 4, Failed: 1, Canceled: 1, Services: 3, AverageMS: 125,
	}})
	summary, err := service.SummaryContext(context.Background(), 24)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SuccessRate != 4.0/6.0*100 || summary.Services != 3 || summary.WindowHours != 24 {
		t.Fatalf("summary = %#v", summary)
	}
	if _, err := service.SummaryContext(context.Background(), 24*91); !errors.Is(err, ErrInvalidTaskRunWindow) {
		t.Fatalf("window error = %v", err)
	}
}
