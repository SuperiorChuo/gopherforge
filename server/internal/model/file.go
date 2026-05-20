package model

import "time"

// File stores uploaded file metadata.
type File struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`
	FileName    string    `gorm:"size:255;not null" json:"file_name"`
	FilePath    string    `gorm:"size:500;not null" json:"file_path"`
	FileSize    int64     `json:"file_size"`
	FileType    string    `gorm:"size:50" json:"file_type"`
	MimeType    string    `gorm:"size:100" json:"mime_type"`
	Extension   string    `gorm:"size:20" json:"extension"`
	StorageType string    `gorm:"size:20;default:'local'" json:"storage_type"`
	URL         string    `gorm:"size:500" json:"url"`
	Hash        string    `gorm:"size:64;index" json:"hash"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
