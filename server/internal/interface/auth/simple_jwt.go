package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
	tokenExpiry   = 24 * time.Hour
)

// SimpleJWTService implements AuthService using argon2id password hashing and HS256 JWT.
type SimpleJWTService struct {
	userRepo  user.UserRepository
	jwtSecret []byte
}

// NewSimpleJWTService creates a new SimpleJWTService.
func NewSimpleJWTService(userRepo user.UserRepository, jwtSecret string) *SimpleJWTService {
	return &SimpleJWTService{
		userRepo:  userRepo,
		jwtSecret: []byte(jwtSecret),
	}
}

// Register creates a new user and returns a token pair.
func (s *SimpleJWTService) Register(ctx context.Context, email, username, password string) (*domainauth.TokenPair, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now()
	u := &user.User{
		ID:           uuid.New().String(),
		Email:        email,
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err = s.userRepo.Create(ctx, u); err != nil {
		return nil, err
	}

	token, err := s.generateToken(u.ID)
	if err != nil {
		return nil, err
	}

	return &domainauth.TokenPair{
		AccessToken: token,
		TokenType:   "Bearer",
	}, nil
}

// Login authenticates a user and returns a token pair.
func (s *SimpleJWTService) Login(ctx context.Context, email, password string) (*domainauth.TokenPair, error) {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	if !verifyPassword(password, u.PasswordHash) {
		return nil, domain.ErrInvalidCredentials
	}

	token, err := s.generateToken(u.ID)
	if err != nil {
		return nil, err
	}

	return &domainauth.TokenPair{
		AccessToken: token,
		TokenType:   "Bearer",
	}, nil
}

// ValidateToken validates a JWT and returns the claims.
func (s *SimpleJWTService) ValidateToken(_ context.Context, tokenString string) (*domainauth.Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, domain.ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, domain.ErrInvalidToken
	}

	sub, err := claims.GetSubject()
	if err != nil {
		return nil, domain.ErrInvalidToken
	}

	return &domainauth.Claims{UserID: sub}, nil
}

func (s *SimpleJWTService) generateToken(userID string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenExpiry)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Time, argon2Threads, saltB64, hashB64), nil
}

func verifyPassword(password, encodedHash string) bool {
	var memory uint32
	var time uint32
	var threads uint8
	var saltB64, hashB64 string

	_, err := fmt.Sscanf(encodedHash, "$argon2id$v=19$m=%d,t=%d,p=%d$%s",
		&memory, &time, &threads, &saltB64)
	if err != nil {
		return false
	}

	// Split saltB64 which contains "salt$hash"
	parts := splitLast(saltB64, '$')
	if parts == nil {
		return false
	}
	saltB64 = parts[0]
	hashB64 = parts[1]

	salt, err := base64.RawStdEncoding.DecodeString(saltB64)
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(hashB64)
	if err != nil {
		return false
	}

	computedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, argon2KeyLen)

	return subtle.ConstantTimeCompare(expectedHash, computedHash) == 1
}

func splitLast(s string, sep byte) []string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}
