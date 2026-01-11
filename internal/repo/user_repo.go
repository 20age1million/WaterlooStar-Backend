package repo

import (
	"context"
	"errors"

	"gorm.io/gorm"

	userdomain "github.com/20age1million/WaterlooStar-Backend/internal/domain/user"
)

type UserRepo interface {
	Create(ctx context.Context, u *userdomain.User) error
	GetByEmail(ctx context.Context, email string) (userdomain.User, error)
	GetByUsername(ctx context.Context, username string) (userdomain.User, error)
	GetByIDs(ctx context.Context, ids []string) (map[string]userdomain.User, error)
	MarkVerified(ctx context.Context, email string) error
}

type GormUserRepo struct {
	db *gorm.DB
}

func NewGormUserRepo(db *gorm.DB) UserRepo {
	return &GormUserRepo{db: db}
}

func (r *GormUserRepo) Create(ctx context.Context, u *userdomain.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *GormUserRepo) GetByEmail(ctx context.Context, email string) (userdomain.User, error) {
	var user userdomain.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	return user, err
}

func (r *GormUserRepo) GetByUsername(ctx context.Context, username string) (userdomain.User, error) {
	var user userdomain.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	return user, err
}

func (r *GormUserRepo) GetByIDs(ctx context.Context, ids []string) (map[string]userdomain.User, error) {
	result := make(map[string]userdomain.User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var users []userdomain.User
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}

	for _, u := range users {
		result[u.ID] = u
	}

	return result, nil
}

func (r *GormUserRepo) MarkVerified(ctx context.Context, email string) error {
	res := r.db.WithContext(ctx).Model(&userdomain.User{}).
		Where("email = ?", email).
		Update("verified", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
