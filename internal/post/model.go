package post

import (
	"time"
)

type Post struct {
	ID    string
	Title string
	Body  string
	//Images []
	CreatorID     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Views         int
	Likes         int
	Stars         int
	CommentNumber int
	CommentID     []string //FKs to comment
}

type Comment struct {
	ID              string
	PostID          string //FK to post
	ParentCommentID string //FK to parent comment, NULL if not exist
	Body            string
	//Images []?
	CreatorID string
	CreatedAt time.Time
	UpdatedAt time.Time
	Likes     int
}
