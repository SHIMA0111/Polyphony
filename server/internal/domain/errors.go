package domain

import "errors"

var (
	// ErrNotFound indicates the requested resource was not found.
	ErrNotFound = errors.New("not found")

	// ErrEmailAlreadyExists indicates a user with the given email already exists.
	ErrEmailAlreadyExists = errors.New("email already exists")

	// ErrUsernameAlreadyExists indicates a user with the given username already exists.
	ErrUsernameAlreadyExists = errors.New("username already exists")

	// ErrInvalidCredentials indicates the provided credentials are invalid.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidToken indicates the provided token is invalid or expired.
	ErrInvalidToken = errors.New("invalid token")

	// ErrUnauthorized indicates the request lacks valid authentication.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the authenticated user lacks permission.
	ErrForbidden = errors.New("forbidden")

	// ErrLLMGateway indicates an error communicating with the LLM Gateway.
	ErrLLMGateway = errors.New("llm gateway error")

	// ErrInvalidMessageType indicates the message type is not valid for the operation.
	ErrInvalidMessageType = errors.New("invalid message type")
)

// IsLLMGatewayError checks if the error wraps ErrLLMGateway.
func IsLLMGatewayError(err error) bool {
	return errors.Is(err, ErrLLMGateway)
}
