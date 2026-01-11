package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	userdomain "github.com/20age1million/WaterlooStar-Backend/internal/domain/user"
	"github.com/20age1million/WaterlooStar-Backend/internal/repo"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicateUsername   = errors.New("duplicate username")
	ErrDuplicateEmail      = errors.New("duplicate email")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidVerification = errors.New("invalid verification code")
)

type AuthService interface {
	Register(ctx context.Context, username, email, password string) error
	Login(ctx context.Context, username, email, password string) (userdomain.User, error)
	SendVerificationCode(ctx context.Context, email string) error
	VerifyCode(ctx context.Context, email, code string) error
}

type authService struct {
	users repo.UserRepo
	codes *VerificationStore
}

func NewAuthService(users repo.UserRepo, codes *VerificationStore) AuthService {
	return &authService{
		users: users,
		codes: codes,
	}
}

func (s *authService) Register(ctx context.Context, username, email, password string) error {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	password = strings.TrimSpace(password)

	if username == "" || email == "" || password == "" {
		return errors.New("username, email, and password are required")
	}

	if _, err := s.users.GetByUsername(ctx, username); err == nil {
		return ErrDuplicateUsername
	} else if !repo.IsNotFound(err) {
		return err
	}

	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return ErrDuplicateEmail
	} else if !repo.IsNotFound(err) {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := userdomain.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
	}

	return s.users.Create(ctx, &user)
}

func (s *authService) Login(ctx context.Context, username, email, password string) (userdomain.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	password = strings.TrimSpace(password)

	if password == "" || (username == "" && email == "") {
		return userdomain.User{}, ErrInvalidCredentials
	}

	var (
		user userdomain.User
		err  error
	)
	if email != "" {
		user, err = s.users.GetByEmail(ctx, email)
	} else {
		user, err = s.users.GetByUsername(ctx, username)
	}
	if err != nil {
		if repo.IsNotFound(err) {
			return userdomain.User{}, ErrInvalidCredentials
		}
		return userdomain.User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return userdomain.User{}, ErrInvalidCredentials
	}

	return user, nil
}

func (s *authService) SendVerificationCode(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return ErrUserNotFound
	}

	if _, err := s.users.GetByEmail(ctx, email); err != nil {
		if repo.IsNotFound(err) {
			return ErrUserNotFound
		}
		return err
	}

	if s.codes == nil {
		return errors.New("verification store not configured")
	}

	code, err := generateVerificationCode()
	if err != nil {
		return err
	}

	s.codes.Set(email, code)
	return nil
}

func (s *authService) VerifyCode(ctx context.Context, email, code string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	code = strings.TrimSpace(code)
	if email == "" || code == "" {
		return ErrInvalidVerification
	}

	if s.codes == nil || !s.codes.Verify(email, code) {
		return ErrInvalidVerification
	}

	if err := s.users.MarkVerified(ctx, email); err != nil {
		if repo.IsNotFound(err) {
			return ErrUserNotFound
		}
		return err
	}

	return nil
}

func generateVerificationCode() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	num := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if num < 0 {
		num = -num
	}
	return fmt.Sprintf("%06d", num%1000000), nil
}
