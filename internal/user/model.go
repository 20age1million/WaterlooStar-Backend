package user

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
