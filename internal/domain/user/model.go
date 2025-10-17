package user

/*
type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	Role         string
	Verified     bool
	//Avator ...
	Level      int
	StarPoints int      // virtual currency
	Follows    []string //list of id
	Followers  []string //list of id
}
*/

type User struct {
	ID           string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Username     string `gorm:"type:varchar(50);not null" json:"username"`
	Email        string `gorm:"type:varchar(100);unique;not null" json:"email"`
	PasswordHash string `gorm:"type:text;not null" json:"-"`
	Role         string `gorm:"type:varchar(20);default:'user'" json:"role"`
	Verified     bool   `gorm:"default:false" json:"verified"`
	//Avatar...
	Level      int     `gorm:"default:1" json:"level"`
	StarPoints int     `gorm:"default:0" json:"star_points"` // virtual currency
	Follows    []*User `gorm:"many2many:user_follows;joinForeignKey:FollowerID;joinReferences:FolloweeID" json:"follows,omitempty"`
	Followers  []*User `gorm:"many2many:user_follows;joinForeignKey:FolloweeID;joinReferences:FollowerID" json:"followers,omitempty"`
}

type UserFollow struct {
	FollowerID string `gorm:"type:uuid;not null;index" json:"follower_id"`
	FolloweeID string `gorm:"type:uuid;not null;index" json:"followee_id"`
}
