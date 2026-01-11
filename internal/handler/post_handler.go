package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/20age1million/WaterlooStar-Backend/internal/repo"
	"github.com/20age1million/WaterlooStar-Backend/internal/service"
)

type PostHandler struct {
	svc service.PostService
}

func NewPostHandler(svc service.PostService) *PostHandler {
	return &PostHandler{svc: svc}
}

type CreatePostRequest struct {
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	CreatorID string   `json:"creator_id"`
	Images    []string `json:"images,omitempty"`
}

type PostListRequest struct {
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
	Sort     map[string]string `json:"sort"`
	Filters  map[string]string `json:"filters"`
}

type PostListResponse struct {
	Meta PostListMeta   `json:"meta"`
	Data []PostListItem `json:"data"`
}

type PostListMeta struct {
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
	Sort       map[string]string `json:"sort"`
	Filters    map[string]string `json:"filters"`
}

type PostListItem struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Excerpt       string         `json:"excerpt"`
	Author        PostListAuthor `json:"author"`
	Image         []string       `json:"image"`
	Stats         PostListStats  `json:"stats"`
	CreatedAt     string         `json:"createdAt"`
	LastUpdatedAt string         `json:"lastUpdatedAt"`
	Flag          PostListFlag   `json:"flag"`
}

type PostListAuthor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PostListStats struct {
	Views   int `json:"views"`
	Likes   int `json:"likes"`
	Stars   int `json:"stars"`
	Replies int `json:"replies"`
}

type PostListFlag struct {
	Liked  bool `json:"liked"`
	Stared bool `json:"stared"`
}

func (h *PostHandler) GetPosts(w http.ResponseWriter, r *http.Request) {
	limit := parseIntDefault(r.URL.Query().Get("limit"), 20)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

	posts, err := h.svc.GetPosts(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "failed to fetch posts", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (h *PostHandler) CreatePost(w http.ResponseWriter, r *http.Request) {
	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	created, err := h.svc.CreatePost(r.Context(), req.Title, req.Body, req.CreatorID, req.Images)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *PostHandler) ListPostsByPage(w http.ResponseWriter, r *http.Request) {
	var req PostListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	filters, err := parsePostListFilters(req.Filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sort := normalizePostListSort(req.Sort)
	query := repo.PostListQuery{
		Limit:   pageSize,
		Offset: offset,
		Filters: filters,
		Sort:    sort,
	}

	result, err := h.svc.GetPostPage(r.Context(), query)
	if err != nil {
		http.Error(w, "failed to fetch posts", http.StatusInternalServerError)
		return
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = int((result.Total + int64(pageSize) - 1) / int64(pageSize))
	}

	data := make([]PostListItem, 0, len(result.Posts))
	for _, p := range result.Posts {
		authorName := ""
		if author, ok := result.Authors[p.CreatorID]; ok {
			authorName = author.Username
		}

		images := make([]string, 0, len(p.Images))
		for _, img := range p.Images {
			images = append(images, img.URL)
		}

		item := PostListItem{
			ID:      p.ID,
			Title:   p.Title,
			Excerpt: excerptFromBody(p.Body),
			Author: PostListAuthor{
				ID:   p.CreatorID,
				Name: authorName,
			},
			Image: images,
			Stats: PostListStats{
				Views:   p.Views,
				Likes:   p.Likes,
				Stars:   p.Stars,
				Replies: p.CommentNumber,
			},
			CreatedAt:     p.CreatedAt.Format(time.RFC3339),
			LastUpdatedAt: p.UpdatedAt.Format(time.RFC3339),
			Flag: PostListFlag{
				Liked:  false,
				Stared: false,
			},
		}
		data = append(data, item)
	}

	resp := PostListResponse{
		Meta: PostListMeta{
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
			Sort:       req.Sort,
			Filters:    req.Filters,
		},
		Data: data,
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parsePostListFilters(filters map[string]string) (repo.PostListFilters, error) {
	result := repo.PostListFilters{}
	if len(filters) == 0 {
		return result, nil
	}

	if v := strings.TrimSpace(filters["time_from"]); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return result, errInvalidFilterTime("time_from")
		}
		result.TimeFrom = &t
	}
	if v := strings.TrimSpace(filters["time_to"]); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return result, errInvalidFilterTime("time_to")
		}
		result.TimeTo = &t
	}

	return result, nil
}

func normalizePostListSort(sort map[string]string) *repo.PostListSort {
	if len(sort) == 0 {
		return nil
	}
	for field, direction := range sort {
		f := normalizeSortField(field)
		if f == "" {
			return nil
		}
		return &repo.PostListSort{
			Field:     f,
			Direction: normalizeSortDirection(direction),
		}
	}
	return nil
}

func normalizeSortField(field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "created_at", "createdat", "time":
		return "created_at"
	case "views":
		return "views"
	case "likes":
		return "likes"
	case "stars":
		return "stars"
	case "comment_number", "comments", "replies":
		return "comment_number"
	default:
		return ""
	}
}

func normalizeSortDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "asc", "ascend", "ascending":
		return "asc"
	default:
		return "desc"
	}
}

func errInvalidFilterTime(field string) error {
	return fmt.Errorf("invalid %s, expected RFC3339", field)
}

func excerptFromBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	line := body
	if idx := strings.IndexByte(body, '\n'); idx >= 0 {
		line = body[:idx]
	}
	if len(line) <= 200 {
		return line
	}
	return line[:200]
}
