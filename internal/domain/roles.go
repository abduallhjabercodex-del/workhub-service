package domain

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

var roleLevel = map[Role]int{
	RoleOwner:  4,
	RoleAdmin:  3,
	RoleMember: 2,
	RoleViewer: 1,
}

func HasMinimumRole(userRole Role, required []Role) bool {
	if len(required) == 0 {
		return true
	}
	userLevel, ok := roleLevel[userRole]
	if !ok {
		return false
	}
	for _, r := range required {
		if reqLevel, ok := roleLevel[r]; ok && userLevel >= reqLevel {
			return true
		}
	}
	return false
}
