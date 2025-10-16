package post

import (
	"time"
)

type Post struct {
	ID            string      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Title         string      `gorm:"type:varchar(255);not null" json:"title"`
	Body          string      `gorm:"type:text; not null" json:"body"`
	Images        []PostImage `gorm:"foreignKey:PostID" json:"images,omitempty"`
	CreatorID     string      `gorm:"type:uuid; not null" json:"creator_id"`
	CreatedAt     time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time   `gorm:"autoUpdateTime" json:"updated_at"`
	Views         int         `gorm:"default:0" json:"views"`
	Likes         int         `gorm:"default:0" json:"likes"`
	Stars         int         `gorm:"default:0" json:"stars"`
	CommentNumber int         `gorm:"default:0" json:"comment_number"`
	Comments      []Comment   `gorm:"foreignKey:PostID" json:"comments,omitempty"`
}

type Comment struct {
	ID              string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PostID          string         `gorm:"type:uuid;not null" json:"post_id"`  // FK to post
	ParentCommentID string         `gorm:"type:uuid" json:"parent_comment_id"` // FK to parent comment, NULL if not exist
	Body            string         `gorm:"type:text;not null" json:"body"`
	Images          []CommentImage `gorm:"foreignKey:CommentID" json:"images,omitempty"`
	CreatorID       string         `gorm:"type:uuid;not null" json:"creator_id"`
	CreatedAt       time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	Likes           int            `gorm:"default:0" json:"likes"`
}

type PostImage struct {
	ID        string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PostID    string    `gorm:"type:uuid;not null;index" json:"post_id"`
	URL       string    `gorm:"type:text;not null" json:"url"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type CommentImage struct {
	ID        string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CommentID string    `gorm:"type:uuid;not null;index" json:"comment_id"`
	URL       string    `gorm:"type:text;not null" json:"url"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
