package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestConsoleRouteDAOListAllOrdersBySortAndKey(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery("SELECT \\* FROM `console_routes` ORDER BY sort_order ASC,route_key ASC").
		WillReturnRows(sqlmock.NewRows([]string{"route_key", "path", "name", "component_key"}).
			AddRow("dashboard", "/dashboard", "Dashboard", "DashboardPage"))

	routes, err := NewConsoleRouteDAO(db).ListAllContext(context.Background())
	if err != nil {
		t.Fatalf("ListAllContext() error = %v", err)
	}
	if len(routes) != 1 || routes[0].RouteKey != "dashboard" {
		t.Fatalf("routes = %#v, want dashboard route", routes)
	}
}

func TestConsoleRouteDAOListAllContextHonorsCanceledContext(t *testing.T) {
	db, _ := newAuthDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewConsoleRouteDAO(db).ListAllContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListAllContext() error = %v, want context.Canceled", err)
	}
}

func TestConsoleRouteDAOFindRouteKeyByPath(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery("SELECT `route_key` FROM `console_routes` WHERE path = \\? LIMIT \\?").
		WithArgs("/dashboard", 1).
		WillReturnRows(sqlmock.NewRows([]string{"route_key"}).AddRow("dashboard"))

	owner, err := NewConsoleRouteDAO(db).FindRouteKeyByPathContext(context.Background(), "/dashboard")
	if err != nil {
		t.Fatalf("FindRouteKeyByPathContext() error = %v", err)
	}
	if owner != "dashboard" {
		t.Fatalf("owner = %q, want dashboard", owner)
	}
}

func TestConsoleRouteDAOUsesInjectedDB(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery("SELECT \\* FROM `console_routes` ORDER BY sort_order ASC,route_key ASC").
		WillReturnRows(sqlmock.NewRows([]string{"route_key", "path", "name", "component_key"}).
			AddRow("dashboard", "/dashboard", "Dashboard", "DashboardPage"))

	routes, err := NewConsoleRouteDAO(db).ListAllContext(context.Background())
	if err != nil {
		t.Fatalf("ListAllContext() error = %v", err)
	}
	if len(routes) != 1 || routes[0].RouteKey != "dashboard" {
		t.Fatalf("routes = %#v, want dashboard route", routes)
	}
}
