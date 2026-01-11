package handler

import (
	"encoding/json"
	"net/http"

	"github.com/20age1million/WaterlooStar-Backend/internal/service"
)

type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type AuthRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type VerificationRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type AuthResponse struct {
	Status string      `json:"status"`
	User   *AuthUser   `json:"user,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type AuthUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	err := h.svc.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		switch err {
		case service.ErrDuplicateUsername:
			writeJSON(w, http.StatusConflict, AuthResponse{Status: "error", Error: "duplicate username"})
		case service.ErrDuplicateEmail:
			writeJSON(w, http.StatusConflict, AuthResponse{Status: "error", Error: "duplicate email"})
		default:
			writeJSON(w, http.StatusBadRequest, AuthResponse{Status: "error", Error: err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{Status: "ok"})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, err := h.svc.Login(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		if err == service.ErrInvalidCredentials {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Error(w, "login failed", http.StatusBadRequest)
		}
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		Status: "ok",
		User: &AuthUser{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Verified: user.Verified,
		},
	})
}

func (h *AuthHandler) SendVerificationCode(w http.ResponseWriter, r *http.Request) {
	var req VerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.svc.SendVerificationCode(r.Context(), req.Email); err != nil {
		if err == service.ErrUserNotFound {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Error(w, "send code failed", http.StatusBadRequest)
		}
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{Status: "ok"})
}

func (h *AuthHandler) VerifyCode(w http.ResponseWriter, r *http.Request) {
	var req VerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.svc.VerifyCode(r.Context(), req.Email, req.Code); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{Status: "ok"})
}
