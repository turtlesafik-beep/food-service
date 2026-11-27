package products

import (
	"errors"
	"fmt"
)

var (
	ErrDishNotFound       = errors.New("dish not found")
	ErrIngredientNotFound = errors.New("ingredient not found")
	ErrCategoryNotFound   = errors.New("category not found")
)

// ValidationError describes a user-facing validation problem.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// ConflictError describes a state conflict, for example deleting an
// ingredient that is still used by a dish.
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return e.Message
}

func validationError(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

func conflictError(format string, args ...any) error {
	return &ConflictError{Message: fmt.Sprintf(format, args...)}
}
