package auth

import (
	"context"
	"errors"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/gorm"
)

var ErrConsoleRouteDatabaseNotInitialized = errors.New("database is not initialized")

type ConsoleRouteDAO struct {
	db *gorm.DB
}

func NewConsoleRouteDAO(dbs ...*gorm.DB) ConsoleRouteDAO {
	db := database.DB
	if len(dbs) > 0 {
		db = dbs[0]
	}
	return ConsoleRouteDAO{db: db}
}

func (d ConsoleRouteDAO) Ready() bool {
	return d.db != nil
}

func (d ConsoleRouteDAO) TransactionContext(ctx context.Context, fn func(ConsoleRouteDAO) error) error {
	if d.db == nil {
		return ErrConsoleRouteDatabaseNotInitialized
	}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ConsoleRouteDAO{db: tx})
	})
}

func (d ConsoleRouteDAO) ListAllContext(ctx context.Context) ([]model.ConsoleRoute, error) {
	var rows []model.ConsoleRoute
	err := d.db.WithContext(ctx).Order("sort_order ASC").Order("route_key ASC").Find(&rows).Error
	return rows, err
}

func (d ConsoleRouteDAO) ListEnabledContext(ctx context.Context) ([]model.ConsoleRoute, error) {
	var rows []model.ConsoleRoute
	err := d.db.WithContext(ctx).Where("enabled = ?", true).Order("sort_order ASC").Order("route_key ASC").Find(&rows).Error
	return rows, err
}

func (d ConsoleRouteDAO) ListPermissionRowsContext(ctx context.Context) ([]model.ConsoleRoute, error) {
	var rows []model.ConsoleRoute
	err := d.db.WithContext(ctx).Select("permissions_json").Find(&rows).Error
	return rows, err
}

func (d ConsoleRouteDAO) GetByRouteKeyContext(ctx context.Context, routeKey string) (*model.ConsoleRoute, error) {
	var route model.ConsoleRoute
	err := d.db.WithContext(ctx).Where("route_key = ?", routeKey).First(&route).Error
	return &route, err
}

func (d ConsoleRouteDAO) CreateContext(ctx context.Context, route *model.ConsoleRoute) error {
	return d.db.WithContext(ctx).Create(route).Error
}

func (d ConsoleRouteDAO) SaveContext(ctx context.Context, route *model.ConsoleRoute) error {
	return d.db.WithContext(ctx).Save(route).Error
}

func (d ConsoleRouteDAO) DeleteContext(ctx context.Context, route *model.ConsoleRoute) error {
	return d.db.WithContext(ctx).Delete(route).Error
}

func (d ConsoleRouteDAO) CountByRouteKeyContext(ctx context.Context, routeKey string) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&model.ConsoleRoute{}).Where("route_key = ?", routeKey).Count(&count).Error
	return count, err
}

func (d ConsoleRouteDAO) FindRouteKeyByPathContext(ctx context.Context, path string) (string, error) {
	return d.findRouteKeyContext(ctx, "path = ?", path)
}

func (d ConsoleRouteDAO) FindRouteKeyByNameContext(ctx context.Context, name string) (string, error) {
	return d.findRouteKeyContext(ctx, "name = ?", name)
}

func (d ConsoleRouteDAO) findRouteKeyContext(ctx context.Context, query string, arg any) (string, error) {
	var routeKey string
	err := d.db.WithContext(ctx).Model(&model.ConsoleRoute{}).Select("route_key").Where(query, arg).Limit(1).Scan(&routeKey).Error
	return routeKey, err
}
