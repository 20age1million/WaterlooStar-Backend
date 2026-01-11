package database

import (
	"gorm.io/gorm"

	postdomain "github.com/20age1million/WaterlooStar-Backend/internal/domain/post"
	userdomain "github.com/20age1million/WaterlooStar-Backend/internal/domain/user"
)

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&postdomain.Post{},
		&postdomain.PostImage{},
		&postdomain.Comment{},
		&postdomain.CommentImage{},
		&userdomain.User{},
		&userdomain.UserFollow{},
	)
}
