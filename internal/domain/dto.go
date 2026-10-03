package domain

// Data Transfer Objects (DTOs) and API Command Inputs.
// Separated from core domain entities to adhere to Clean Architecture boundaries.

type CreateProjectInput struct {
	Name string `json:"name"`
}

type CreateTaskInput struct {
	Title      string  `json:"title"`
	AssigneeID *string `json:"assigneeId,omitempty"`
}

type UpdateTaskInput struct {
	Title      *string `json:"title"`
	Status     *string `json:"status"`
	AssigneeID *string `json:"assigneeId,omitempty"`
}

type RegisterInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshInput struct {
	RefreshToken string `json:"refreshToken"`
}

type LogoutInput struct {
	RefreshToken string `json:"refreshToken"`
}

type AuthResponse struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	User         AuthUser `json:"user"`
}

type AddMemberInput struct {
	UserID string `json:"userId"`
	Role   Role   `json:"role"`
}
