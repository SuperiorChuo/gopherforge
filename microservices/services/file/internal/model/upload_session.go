package localmodel

import "time"

// UploadSession tracks a chunked upload: which parts have arrived and where
// the assembled object will land. Completed sessions are deleted; expired ones
// are pruned lazily on init.
type UploadSession struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	TenantID       uint      `gorm:"not null;default:1;index" json:"tenant_id"`
	UserID         uint      `gorm:"not null;index" json:"user_id"`
	FileName       string    `gorm:"size:255;not null" json:"file_name"`
	FileSize       int64     `gorm:"not null" json:"file_size"`
	ChunkSize      int64     `gorm:"not null" json:"chunk_size"`
	TotalChunks    int       `gorm:"not null" json:"total_chunks"`
	ReceivedCount  int       `gorm:"not null;default:0" json:"received_count"`
	ReceivedBitmap string    `gorm:"size:1024;not null;default:'[]'" json:"received_bitmap"`
	StorageType    string    `gorm:"size:20;not null;default:'local'" json:"storage_type"`
	ObjectKey      string    `gorm:"size:512;not null;default:''" json:"object_key"`
	Hash           string    `gorm:"size:64;not null;default:''" json:"hash"`
	Status         string    `gorm:"size:16;not null;default:'pending'" json:"status"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (UploadSession) TableName() string { return "upload_sessions" }
