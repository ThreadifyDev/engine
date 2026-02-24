package tests

import apperrors "github.com/threadify/engine/internal/utils/errors"

type ErrorType = apperrors.ErrorType
type AppError = apperrors.AppError

const (
	ErrorTypeNotFound     = apperrors.ErrorTypeNotFound
	ErrorTypeValidation   = apperrors.ErrorTypeValidation
	ErrorTypeUnauthorized = apperrors.ErrorTypeUnauthorized
	ErrorTypeForbidden    = apperrors.ErrorTypeForbidden
	ErrorTypeConflict     = apperrors.ErrorTypeConflict
	ErrorTypeInternal     = apperrors.ErrorTypeInternal
	ErrorTypeBadRequest   = apperrors.ErrorTypeBadRequest
	ErrorTypeUnavailable  = apperrors.ErrorTypeUnavailable
)

func NewNotFoundError(message string, err error) *AppError {
	return apperrors.NewNotFoundError(message, err)
}

func NewValidationError(message string, err error) *AppError {
	return apperrors.NewValidationError(message, err)
}

func NewUnauthorizedError(message string, err error) *AppError {
	return apperrors.NewUnauthorizedError(message, err)
}

func NewForbiddenError(message string, err error) *AppError {
	return apperrors.NewForbiddenError(message, err)
}

func NewConflictError(message string, err error) *AppError {
	return apperrors.NewConflictError(message, err)
}

func NewBadRequestError(message string, err error) *AppError {
	return apperrors.NewBadRequestError(message, err)
}

func NewInternalError(message string, err error) *AppError {
	return apperrors.NewInternalError(message, err)
}

func NewUnavailableError(message string, err error) *AppError {
	return apperrors.NewUnavailableError(message, err)
}

func IsNotFound(err error) bool {
	return apperrors.IsNotFound(err)
}

func IsValidation(err error) bool {
	return apperrors.IsValidation(err)
}

func GetStatusCode(err error) int {
	return apperrors.GetStatusCode(err)
}

func GetUserMessage(err error) string {
	return apperrors.GetUserMessage(err)
}

func GetErrorType(err error) ErrorType {
	return apperrors.GetErrorType(err)
}
