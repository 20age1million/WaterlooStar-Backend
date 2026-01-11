package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	postdomain "github.com/20age1million/WaterlooStar-Backend/internal/domain/post"
)

type PostRepo interface {
	List(ctx context.Context, limit, offset int) ([]postdomain.Post, error)
	Create(ctx context.Context, p *postdomain.Post) error
	ListPage(ctx context.Context, q PostListQuery) (PostListResult, error)
}

type GormPostRepo struct {
	db *gorm.DB
}

func NewGormPostRepo(db *gorm.DB) PostRepo {
	return &GormPostRepo{db: db}
}

func (r *GormPostRepo) List(ctx context.Context, limit, offset int) ([]postdomain.Post, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var posts []postdomain.Post
	err := r.db.WithContext(ctx).
		Preload("Images").
		Preload("Comments").
		Preload("Comments.Images").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&posts).Error

	return posts, err
}

func (r *GormPostRepo) Create(ctx context.Context, p *postdomain.Post) error {
	return r.db.WithContext(ctx).Create(p).Error
}

type PostListFilters struct {
	TimeFrom *time.Time
	TimeTo   *time.Time
}

type PostListSort struct {
	Field     string
	Direction string
}

type PostListQuery struct {
	Limit   int
	Offset  int
	Filters PostListFilters
	Sort    *PostListSort
}

type PostListResult struct {
	Posts []postdomain.Post
	Total int64
}

func (r *GormPostRepo) ListPage(ctx context.Context, q PostListQuery) (PostListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	base := r.db.WithContext(ctx).Model(&postdomain.Post{}).Preload("Images")

	if q.Filters.TimeFrom != nil {
		base = base.Where("created_at >= ?", *q.Filters.TimeFrom)
	}
	if q.Filters.TimeTo != nil {
		base = base.Where("created_at <= ?", *q.Filters.TimeTo)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return PostListResult{}, err
	}

	order := "created_at DESC"
	if q.Sort != nil && q.Sort.Field != "" {
		direction := "DESC"
		if q.Sort.Direction == "asc" {
			direction = "ASC"
		}
		switch q.Sort.Field {
		case "created_at", "views", "likes", "stars", "comment_number":
			order = q.Sort.Field + " " + direction
		}
	}

	var posts []postdomain.Post
	err := base.
		Order(order).
		Limit(q.Limit).
		Offset(q.Offset).
		Find(&posts).Error
	if err != nil {
		return PostListResult{}, err
	}

	return PostListResult{Posts: posts, Total: total}, nil
}
