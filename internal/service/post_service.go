package service

import (
	"context"
	"errors"
	"strings"

	postdomain "github.com/20age1million/WaterlooStar-Backend/internal/domain/post"
	userdomain "github.com/20age1million/WaterlooStar-Backend/internal/domain/user"
	"github.com/20age1million/WaterlooStar-Backend/internal/repo"
)

type PostService interface {
	GetPosts(ctx context.Context, limit, offset int) ([]postdomain.Post, error)
	CreatePost(ctx context.Context, title, body, creatorID string, imageURLs []string) (postdomain.Post, error)
	GetPostPage(ctx context.Context, q repo.PostListQuery) (PostPageResult, error)
}

type postService struct {
	repo     repo.PostRepo
	userRepo repo.UserRepo
}

type PostPageResult struct {
	Posts   []postdomain.Post
	Total   int64
	Authors map[string]userdomain.User
}

func NewPostService(r repo.PostRepo, userRepo repo.UserRepo) PostService {
	return &postService{repo: r, userRepo: userRepo}
}

func (s *postService) GetPosts(ctx context.Context, limit, offset int) ([]postdomain.Post, error) {
	return s.repo.List(ctx, limit, offset)
}

func (s *postService) CreatePost(
	ctx context.Context,
	title, body, creatorID string,
	imageURLs []string,
) (postdomain.Post, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	creatorID = strings.TrimSpace(creatorID)

	if title == "" {
		return postdomain.Post{}, errors.New("title is required")
	}
	if body == "" {
		return postdomain.Post{}, errors.New("body is required")
	}
	if creatorID == "" {
		return postdomain.Post{}, errors.New("creator_id is required")
	}

	p := postdomain.Post{
		Title:     title,
		Body:      body,
		CreatorID: creatorID,
	}

	if len(imageURLs) > 0 {
		p.Images = make([]postdomain.PostImage, 0, len(imageURLs))
		for _, url := range imageURLs {
			u := strings.TrimSpace(url)
			if u == "" {
				continue
			}
			p.Images = append(p.Images, postdomain.PostImage{URL: u})
		}
	}

	if err := s.repo.Create(ctx, &p); err != nil {
		return postdomain.Post{}, err
	}

	return p, nil
}

func (s *postService) GetPostPage(ctx context.Context, q repo.PostListQuery) (PostPageResult, error) {
	result, err := s.repo.ListPage(ctx, q)
	if err != nil {
		return PostPageResult{}, err
	}

	ids := make([]string, 0, len(result.Posts))
	seen := make(map[string]struct{}, len(result.Posts))
	for _, p := range result.Posts {
		if p.CreatorID == "" {
			continue
		}
		if _, ok := seen[p.CreatorID]; ok {
			continue
		}
		seen[p.CreatorID] = struct{}{}
		ids = append(ids, p.CreatorID)
	}

	authors := map[string]userdomain.User{}
	if s.userRepo != nil && len(ids) > 0 {
		authors, err = s.userRepo.GetByIDs(ctx, ids)
		if err != nil {
			return PostPageResult{}, err
		}
	}

	return PostPageResult{
		Posts:   result.Posts,
		Total:   result.Total,
		Authors: authors,
	}, nil
}
