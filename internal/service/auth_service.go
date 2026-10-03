package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/auth"
	"workhub-service/internal/infrastructure/telemetry"
)

func (s *WorkHub) Register(ctx context.Context, in domain.RegisterInput) (domain.AuthResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.Register")
	defer span.End()

	email := strings.ToLower(strings.TrimSpace(in.Email))
	displayName := strings.TrimSpace(in.DisplayName)
	password := in.Password

	if email == "" || len(email) > 254 {
		return domain.AuthResponse{}, ErrInvalidInput
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return domain.AuthResponse{}, ErrInvalidInput
	}
	if displayName == "" || len(password) < 12 || len(password) > 72 {
		return domain.AuthResponse{}, ErrInvalidInput
	}
	if password == email || password == displayName {
		return domain.AuthResponse{}, ErrInvalidInput
	}
	if s.users == nil {
		return domain.AuthResponse{}, ErrStorage
	}

	hash, err := auth.HashPassword(password, s.bcryptCost)
	if err != nil {
		return domain.AuthResponse{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.users.CreateUser(ctx, email, displayName, hash)
	if err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return domain.AuthResponse{}, ErrUserAlreadyExists
		}
		return domain.AuthResponse{}, fmt.Errorf("create user: %w", err)
	}
	resp, err := s.generateTokens(ctx, user)
	if err == nil {
		slog.InfoContext(ctx, "user registered successfully", "user_id", user.ID, "email", user.Email)
	}
	return resp, err
}

func (s *WorkHub) Login(ctx context.Context, in domain.LoginInput) (domain.AuthResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.Login")
	defer span.End()

	email := strings.ToLower(strings.TrimSpace(in.Email))
	password := in.Password
	if email == "" || password == "" {
		return domain.AuthResponse{}, ErrInvalidInput
	}
	if s.users == nil {
		return domain.AuthResponse{}, ErrStorage
	}
	user, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		slog.WarnContext(ctx, "login failed: invalid credentials", "email", email)
		return domain.AuthResponse{}, ErrInvalidCredentials
	}
	if !auth.CheckPasswordHash(password, user.PasswordHash) {
		slog.WarnContext(ctx, "login failed: invalid credentials", "user_id", user.ID)
		return domain.AuthResponse{}, ErrInvalidCredentials
	}
	resp, err := s.generateTokens(ctx, user)
	if err == nil {
		slog.InfoContext(ctx, "login successful", "user_id", user.ID)
	}
	return resp, err
}

func (s *WorkHub) RefreshToken(ctx context.Context, refreshTokenStr string) (domain.AuthResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.RefreshToken")
	defer span.End()

	tokenStr := strings.TrimSpace(refreshTokenStr)
	if tokenStr == "" || s.auth == nil || s.users == nil {
		return domain.AuthResponse{}, ErrInvalidInput
	}
	tokenHash := auth.HashToken(tokenStr)
	newRawRefresh, newHashRefresh, err := auth.GenerateOpaqueToken()
	if err != nil {
		return domain.AuthResponse{}, fmt.Errorf("generate refresh token: %w", err)
	}

	session, err := s.auth.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.AuthResponse{}, ErrUnauthorized
		}
		return domain.AuthResponse{}, fmt.Errorf("get refresh token: %w", err)
	}

	newSessionReq := domain.RefreshTokenSession{
		TokenHash: newHashRefresh,
		UserID:    session.UserID,
		ExpiresAt: s.now().UTC().Add(s.refreshTokenTTL),
	}

	rotatedSession, err := s.auth.RotateRefreshToken(ctx, tokenHash, newSessionReq)
	if err != nil {
		if errors.Is(err, domain.ErrTokenReplay) {
			slog.WarnContext(ctx, "token replay attack detected! revoking all user sessions", "user_id", session.UserID)
			return domain.AuthResponse{}, ErrUnauthorized
		}
		if errors.Is(err, domain.ErrNotFound) {
			return domain.AuthResponse{}, ErrUnauthorized
		}
		return domain.AuthResponse{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	user, err := s.users.GetUserByID(ctx, rotatedSession.UserID)
	if err != nil {
		return domain.AuthResponse{}, ErrUnauthorized
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, s.jwtSecret, s.accessTokenTTL)
	if err != nil {
		return domain.AuthResponse{}, fmt.Errorf("generate access token: %w", err)
	}

	slog.InfoContext(ctx, "token refreshed successfully", "user_id", user.ID)
	return domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRawRefresh,
		User: domain.AuthUser{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
		},
	}, nil
}

func (s *WorkHub) Logout(ctx context.Context, refreshTokenStr string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.Logout")
	defer span.End()

	tokenStr := strings.TrimSpace(refreshTokenStr)
	if tokenStr == "" || s.auth == nil {
		return nil
	}
	tokenHash := auth.HashToken(tokenStr)
	if err := s.auth.RevokeRefreshToken(ctx, tokenHash, nil); err != nil {
		slog.WarnContext(ctx, "failed to revoke refresh token on logout", "error", err)
	} else {
		slog.InfoContext(ctx, "logout successful")
	}
	return nil
}

func (s *WorkHub) GetUserByID(ctx context.Context, actorUserID, targetID string) (domain.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.GetUserByID")
	defer span.End()

	if actorUserID == "" {
		return domain.User{}, ErrUnauthorized
	}
	if actorUserID != targetID {
		return domain.User{}, ErrForbidden
	}
	if s.users == nil {
		return domain.User{}, ErrStorage
	}
	u, err := s.users.GetUserByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (s *WorkHub) generateTokens(ctx context.Context, user domain.User) (domain.AuthResponse, error) {
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, s.jwtSecret, s.accessTokenTTL)
	if err != nil {
		return domain.AuthResponse{}, fmt.Errorf("generate access token: %w", err)
	}
	rawRefresh, hashRefresh, err := auth.GenerateOpaqueToken()
	if err != nil {
		return domain.AuthResponse{}, fmt.Errorf("generate refresh token: %w", err)
	}
	if s.auth != nil {
		session := domain.RefreshTokenSession{
			TokenHash: hashRefresh,
			UserID:    user.ID,
			ExpiresAt: s.now().UTC().Add(s.refreshTokenTTL),
		}
		if err := s.auth.SaveRefreshToken(ctx, session); err != nil {
			return domain.AuthResponse{}, fmt.Errorf("save refresh token: %w", err)
		}
	}
	return domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		User: domain.AuthUser{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
		},
	}, nil
}
