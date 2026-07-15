package shared

import (
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Dependencies carries the runtime infrastructure handles injected at the
// route composition root. Zero value keeps legacy global fallbacks active.
type Dependencies struct {
	DB    *gorm.DB
	Redis *goredis.Client
}
