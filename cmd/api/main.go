package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var db *sql.DB

type apiResponse struct {
	Code    int            `json:"code"`
	Success bool           `json:"success"`
	Message string         `json:"message,omitempty"`
	Data    any            `json:"data,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Remember bool   `json:"remember"`
}

type userAuth struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type authSession struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      userAuth  `json:"user"`
}

type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]authSession
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: make(map[string]authSession),
	}
}

func (s *sessionStore) set(session authSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.Token] = session
}

func (s *sessionStore) get(token string) (authSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[token]
	return session, ok
}

func (s *sessionStore) delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

var (
	demoUser     = userAuth{ID: "user_demo_1", Username: "demo", Email: "demo@waterloo.star"}
	demoPassword = "password123"
	store        = newSessionStore()
)

func main() {
	// Initialize Postgres connection
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	var err error
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	log.Println("Connected to Postgres successfully")

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://127.0.0.1:3000"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	api := router.Group("/api")
	auth := api.Group("/auth")
	auth.POST("/login", loginHandler)
	auth.GET("/me", meHandler)
	api.GET("/health/db", dbHealthHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := router.Run(":" + port); err != nil {
		panic(err)
	}

}
func dbHealthHandler(c *gin.Context) {
	if db == nil {
		respond(c, http.StatusInternalServerError, "DB not initialized", nil)
		return
	}
	if err := db.Ping(); err != nil {
		respond(c, http.StatusInternalServerError, "DB ping failed", map[string]any{"error": err.Error()})
		return
	}
	respond(c, http.StatusOK, "DB OK", nil)
}

func loginHandler(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond(c, http.StatusBadRequest, "Invalid login payload", nil)
		return
	}

	if !strings.EqualFold(request.Email, demoUser.Email) || request.Password != demoPassword {
		respond(c, http.StatusUnauthorized, "Invalid email or password", nil)
		return
	}

	token, err := generateToken()
	if err != nil {
		respond(c, http.StatusInternalServerError, "Unable to create session", nil)
		return
	}

	sessionDuration := 24 * time.Hour
	if request.Remember {
		sessionDuration = 7 * 24 * time.Hour
	}

	session := authSession{
		Token:     token,
		ExpiresAt: time.Now().Add(sessionDuration),
		User:      demoUser,
	}
	store.set(session)

	respond(c, http.StatusOK, "Login successful", session)
}

func meHandler(c *gin.Context) {
	token := extractBearerToken(c.GetHeader("Authorization"))
	if token == "" {
		respond(c, http.StatusUnauthorized, "Missing bearer token", nil)
		return
	}

	session, ok := store.get(token)
	if !ok {
		respond(c, http.StatusUnauthorized, "Invalid or expired token", nil)
		return
	}

	if time.Now().After(session.ExpiresAt) {
		store.delete(token)
		respond(c, http.StatusUnauthorized, "Session expired", nil)
		return
	}

	respond(c, http.StatusOK, "Session active", session.User)
}

func respond(c *gin.Context, status int, message string, data any) {
	c.JSON(status, apiResponse{
		Code:    status,
		Success: status < http.StatusBadRequest,
		Message: message,
		Data:    data,
	})
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func extractBearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
