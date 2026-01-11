package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/20age1million/WaterlooStar-Backend/internal/handler"
)

func New(postHandler *handler.PostHandler, authHandler *handler.AuthHandler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	r.Get("/posts", postHandler.GetPosts)
	r.Post("/posts", postHandler.CreatePost)
	r.Post("/posts/list", postHandler.ListPostsByPage)

	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/login", authHandler.Login)
	r.Post("/auth/send-code", authHandler.SendVerificationCode)
	r.Post("/auth/verify-code", authHandler.VerifyCode)

	return r
}
