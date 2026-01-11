package main

import (
	"log"
	"net/http"
	"time"

	"github.com/20age1million/WaterlooStar-Backend/internal/database"
	"github.com/20age1million/WaterlooStar-Backend/internal/handler"
	"github.com/20age1million/WaterlooStar-Backend/internal/repo"
	"github.com/20age1million/WaterlooStar-Backend/internal/router"
	"github.com/20age1million/WaterlooStar-Backend/internal/service"
)

func main() {
	// 1) Open DB (uses PG_DSN and .env automatically)
	db, err := database.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := database.Close(db); err != nil {
			log.Println("failed to close db:", err)
		}
	}()

	// 2) Auto migrate tables
	if err := database.AutoMigrate(db); err != nil {
		log.Fatal(err)
	}

	// 3) Wire dependencies: repo -> service -> handler -> router
	postRepo := repo.NewGormPostRepo(db)
	userRepo := repo.NewGormUserRepo(db)
	postSvc := service.NewPostService(postRepo, userRepo)
	postHandler := handler.NewPostHandler(postSvc)

	verificationStore := service.NewVerificationStore(10 * time.Minute)
	authSvc := service.NewAuthService(userRepo, verificationStore)
	authHandler := handler.NewAuthHandler(authSvc)

	r := router.New(postHandler, authHandler)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
