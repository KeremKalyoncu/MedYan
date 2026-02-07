package errors

import (
	"errors"
	"fmt"
)

// CustomError represents an application error with metadata
type CustomError struct {
	Code       string      // Machine-readable error code
	Message    string      // Human-readable message
	StatusCode int         // HTTP status code
	Cause      error       // Underlying error
	Details    interface{} // Additional error details
}

// Error implements the error interface
func (e *CustomError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap implements the errors.Unwrap interface for wrapping errors
func (e *CustomError) Unwrap() error {
	return e.Cause
}

// Is checks if an error is of a specific type
func (e *CustomError) Is(target error) bool {
	t, ok := target.(*CustomError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// NewCustomError creates a new custom error
func NewCustomError(code string, message string, statusCode int) *CustomError {
	return &CustomError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
	}
}

// WithCause adds an underlying error
func (e *CustomError) WithCause(err error) *CustomError {
	e.Cause = err
	return e
}

// WithDetails adds additional error details
func (e *CustomError) WithDetails(details interface{}) *CustomError {
	e.Details = details
	return e
}

// Pre-defined errors
var (
	// Validation errors (400)
	ErrInvalidURL = NewCustomError(
		"INVALID_URL",
		"The provided URL is invalid or not supported",
		400,
	)

	ErrInvalidRequest = NewCustomError(
		"INVALID_REQUEST",
		"Request body is invalid or missing required fields",
		400,
	)

	ErrInvalidQuality = NewCustomError(
		"INVALID_QUALITY",
		"The specified quality is not supported",
		400,
	)

	ErrInvalidFormat = NewCustomError(
		"INVALID_FORMAT",
		"The specified format is not supported",
		400,
	)

	// Not found errors (404)
	ErrJobNotFound = NewCustomError(
		"JOB_NOT_FOUND",
		"The requested job was not found",
		404,
	)

	// Conflict errors (409)
	ErrDuplicateJob = NewCustomError(
		"DUPLICATE_JOB",
		"A job for this content already exists",
		409,
	)

	// Rate limiting (429)
	ErrRateLimited = NewCustomError(
		"RATE_LIMITED",
		"Too many requests. Please try again later",
		429,
	)

	// Server errors (500)
	ErrInternal = NewCustomError(
		"INTERNAL_ERROR",
		"An internal server error occurred",
		500,
	)

	ErrQueueFailed = NewCustomError(
		"QUEUE_ERROR",
		"Failed to queue extraction job",
		500,
	)

	ErrStorageFailed = NewCustomError(
		"STORAGE_ERROR",
		"Storage operation failed",
		500,
	)

	ErrExtractionFailed = NewCustomError(
		"EXTRACTION_ERROR",
		"Media extraction failed",
		500,
	)

	ErrDownloadFailed = NewCustomError(
		"DOWNLOAD_ERROR",
		"Failed to download media",
		500,
	)

	ErrProcessingFailed = NewCustomError(
		"PROCESSING_ERROR",
		"Failed to process media",
		500,
	)

	ErrUploadFailed = NewCustomError(
		"UPLOAD_ERROR",
		"Failed to upload processed media",
		500,
	)

	ErrConfigInvalid = NewCustomError(
		"CONFIG_ERROR",
		"Configuration is invalid",
		500,
	)
)

// IsCustomError checks if an error is a CustomError
func IsCustomError(err error) bool {
	var customErr *CustomError
	return errors.As(err, &customErr)
}

// GetStatusCode extracts HTTP status code from an error
func GetStatusCode(err error) int {
	var customErr *CustomError
	if errors.As(err, &customErr) {
		return customErr.StatusCode
	}
	return 500 // Default to internal server error
}

// GetErrorCode extracts error code from an error
func GetErrorCode(err error) string {
	var customErr *CustomError
	if errors.As(err, &customErr) {
		return customErr.Code
	}
	return "UNKNOWN_ERROR"
}

// GetErrorMessage extracts human-readable message from an error
func GetErrorMessage(err error) string {
	var customErr *CustomError
	if errors.As(err, &customErr) {
		return customErr.Message
	}
	return "An unknown error occurred"
}
