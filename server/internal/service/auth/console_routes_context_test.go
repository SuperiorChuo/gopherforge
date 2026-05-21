package auth

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestConsoleRouteServiceListRoutesContextHonorsCanceledContext(t *testing.T) {
	setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (ConsoleRouteService{}).ListRoutesContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListRoutesContext() error = %v, want context.Canceled", err)
	}
}

func TestUniqueSortedConsoleStringsTrimsDeduplicatesAndSorts(t *testing.T) {
	got := UniqueSortedConsoleStrings([]string{" logs.read ", "", "rbac.write", "logs.read", "dashboard.view"})
	want := []string{"dashboard.view", "logs.read", "rbac.write"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UniqueSortedConsoleStrings() = %#v, want %#v", got, want)
	}
}
