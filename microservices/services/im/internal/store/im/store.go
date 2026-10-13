package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	connpool "github.com/go-admin-kit/services/shared/pkg/connpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"strings"
)

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	connpool.Apply(db)
	return NewWithDB(db)
}

// NewWithDB 包装一个已存在的 gorm.DB（测试注入 sqlite 内存库）。
func NewWithDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.seed(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

// Ping 验证底层 SQL 连接是否可达。
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("database is not configured")
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func JSONText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func OriginAllowed(allowedJSON, origin string) bool {
	if origin == "" {
		return true // 非浏览器客户端
	}
	var list []string
	if err := json.Unmarshal([]byte(allowedJSON), &list); err != nil {
		// 容忍配置错误的遗留行：拒绝
		return false
	}
	if len(list) == 0 {
		return true
	}
	for _, o := range list {
		o = strings.TrimSpace(o)
		if o == "*" {
			return true
		}
		if strings.EqualFold(o, strings.TrimSpace(origin)) {
			return true
		}
	}
	return false
}

func FmtErr(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
