package store

import (
	"errors"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Call struct {
	ID          uint64     `gorm:"primaryKey" json:"id"`
	CallID      string     `gorm:"size:64;uniqueIndex" json:"call_id"`
	Direction   string     `gorm:"size:16" json:"direction"`
	Caller      string     `gorm:"size:64" json:"caller"`
	Callee      string     `gorm:"size:64" json:"callee"`
	AgentExt    string     `gorm:"size:32" json:"agent_ext"`
	Queue       string     `gorm:"size:64" json:"queue"`
	Status      string     `gorm:"size:32" json:"status"`
	DurationSec int        `json:"duration_sec"`
	Recording   string     `gorm:"size:512" json:"recording"`
	RawEvent    string     `gorm:"type:text" json:"raw_event,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	AnsweredAt  *time.Time `json:"answered_at,omitempty"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Call) TableName() string { return "cc_calls" }

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := db.AutoMigrate(&Call{}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) UpsertCall(c *Call) error {
	var existing Call
	err := s.db.Where("call_id = ?", c.CallID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(c).Error
	}
	if err != nil {
		return err
	}
	c.ID = existing.ID
	return s.db.Save(c).Error
}

func (s *Store) ListCalls(limit int) ([]Call, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []Call
	err := s.db.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}
