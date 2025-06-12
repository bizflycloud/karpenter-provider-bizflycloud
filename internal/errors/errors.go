package errors

import (
	"errors"
	"fmt"
)

// CloudProvider specific errors
var (
	ErrInstanceNotFound       = errors.New("instance not found")
	ErrNodeClassNotFound      = errors.New("node class not found")
	ErrInvalidProviderID      = errors.New("invalid provider ID format")
	ErrInstanceCreationFailed = errors.New("instance creation failed")
	ErrInstanceDeletionFailed = errors.New("instance deletion failed")
	ErrNodeClassTerminating   = errors.New("node class is terminating")
	ErrInvalidInstanceType    = errors.New("invalid instance type")
	ErrAuthenticationFailed   = errors.New("authentication failed")
	ErrQuotaExceeded          = errors.New("quota exceeded")
	ErrRateLimited            = errors.New("rate limited")
)

// NodeCreationError represents an error during node creation
type NodeCreationError struct {
	NodeName   string
	InstanceID string
	Underlying error
	Retryable  bool
}

func (e *NodeCreationError) Error() string {
	return fmt.Sprintf("failed to create node %s (instance: %s): %v", e.NodeName, e.InstanceID, e.Underlying)
}

func (e *NodeCreationError) Unwrap() error {
	return e.Underlying
}

func (e *NodeCreationError) IsRetryable() bool {
	return e.Retryable
}

// NodeDeletionError represents an error during node deletion
type NodeDeletionError struct {
	NodeName   string
	InstanceID string
	Underlying error
	Retryable  bool
}

func (e *NodeDeletionError) Error() string {
	return fmt.Sprintf("failed to delete node %s (instance: %s): %v", e.NodeName, e.InstanceID, e.Underlying)
}

func (e *NodeDeletionError) Unwrap() error {
	return e.Underlying
}

func (e *NodeDeletionError) IsRetryable() bool {
	return e.Retryable
}

// DriftDetectionError represents an error during drift detection
type DriftDetectionError struct {
	NodeName   string
	Underlying error
}

func (e *DriftDetectionError) Error() string {
	return fmt.Sprintf("failed to detect drift for node %s: %v", e.NodeName, e.Underlying)
}

func (e *DriftDetectionError) Unwrap() error {
	return e.Underlying
}

// IsRetryableError determines if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var nodeCreationErr *NodeCreationError
	if errors.As(err, &nodeCreationErr) {
		return nodeCreationErr.IsRetryable()
	}

	var nodeDeletionErr *NodeDeletionError
	if errors.As(err, &nodeDeletionErr) {
		return nodeDeletionErr.IsRetryable()
	}

	// Check for specific error types
	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrQuotaExceeded) {
		return true
	}

	// Check error message for common retryable patterns
	errMsg := err.Error()
	retryablePatterns := []string{
		"timeout",
		"connection",
		"temporary",
		"rate limit",
		"quota",
		"throttle",
	}

	for _, pattern := range retryablePatterns {
		if containsIgnoreCase(errMsg, pattern) {
			return true
		}
	}

	return false
}

// IsNotFoundError determines if an error represents a "not found" condition
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrInstanceNotFound) || errors.Is(err, ErrNodeClassNotFound) {
		return true
	}

	errMsg := err.Error()
	notFoundPatterns := []string{
		"not found",
		"does not exist",
		"404",
	}

	for _, pattern := range notFoundPatterns {
		if containsIgnoreCase(errMsg, pattern) {
			return true
		}
	}

	return false
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					func() bool {
						for i := 0; i <= len(s)-len(substr); i++ {
							if s[i:i+len(substr)] == substr {
								return true
							}
						}
						return false
					}())))
}
