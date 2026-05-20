package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestConsoleRouteDAOListAllOrdersBySortAndKey(t *testing.T) {
	mock := setupAuthDAOTestDB(t)
	mock.ExpectQuery("SELECT \\* FROM `wm_console_route` ORDER BY sort_order ASC,route_key ASC").
		WillReturnRows(sqlmock.NewRows([]string{"route_key", "path", "name", "component_key"}).
			AddRow("dashboard", "/dashboard", "Dashboard", "DashboardPage"))

	routes, err := NewConsoleRouteDAO().ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(routes) != 1 || routes[0].RouteKey != "dashboard" {
		t.Fatalf("routes = %#v, want dashboard route", routes)
	}
}

func TestConsoleRouteDAOListAllContextHonorsCanceledContext(t *testing.T) {
	setupAuthDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewConsoleRouteDAO().ListAllContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListAllContext() error = %v, want context.Canceled", err)
	}
}

func TestConsoleRouteDAOFindRouteKeyByPath(t *testing.T) {
	mock := setupAuthDAOTestDB(t)
	mock.ExpectQuery("SELECT `route_key` FROM `wm_console_route` WHERE path = \\? LIMIT \\?").
		WithArgs("/dashboard", 1).
		WillReturnRows(sqlmock.NewRows([]string{"route_key"}).AddRow("dashboard"))

	owner, err := NewConsoleRouteDAO().FindRouteKeyByPath("/dashboard")
	if err != nil {
		t.Fatalf("FindRouteKeyByPath() error = %v", err)
	}
	if owner != "dashboard" {
		t.Fatalf("owner = %q, want dashboard", owner)
	}
}
