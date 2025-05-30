package logging

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Logger wraps logr.Logger with additional structured logging capabilities
type Logger struct {
	logr.Logger
}

// NewLogger creates a new structured logger
func NewLogger(name string) *Logger {
	return &Logger{
		Logger: log.Log.WithName(name),
	}
}

// FromContext returns a logger from context
func FromContext(ctx context.Context) *Logger {
	return &Logger{
		Logger: log.FromContext(ctx),
	}
}

// WithValues returns a logger with additional key-value pairs
func (l *Logger) WithValues(keysAndValues ...interface{}) *Logger {
	return &Logger{
		Logger: l.Logger.WithValues(keysAndValues...),
	}
}

// WithName returns a logger with a name suffix
func (l *Logger) WithName(name string) *Logger {
	return &Logger{
		Logger: l.Logger.WithName(name),
	}
}

// NodeCreation logs node creation events
func (l *Logger) NodeCreation(nodeName, instanceType, zone string, duration string) {
	l.Info("Node creation completed",
		"event", "node_creation",
		"nodeName", nodeName,
		"instanceType", instanceType,
		"zone", zone,
		"duration", duration,
	)
}

// NodeDeletion logs node deletion events
func (l *Logger) NodeDeletion(nodeName, instanceID string, duration string) {
	l.Info("Node deletion completed",
		"event", "node_deletion",
		"nodeName", nodeName,
		"instanceID", instanceID,
		"duration", duration,
	)
}

// DriftDetection logs drift detection events
func (l *Logger) DriftDetection(nodeName string, drifted bool, reason string) {
	l.Info("Drift detection completed",
		"event", "drift_detection",
		"nodeName", nodeName,
		"drifted", drifted,
		"reason", reason,
	)
}

// APICall logs API calls to external services
func (l *Logger) APICall(operation, service string, duration string, success bool) {
	level := "info"
	if !success {
		level = "error"
	}

	if level == "error" {
		l.Error(nil, "API call completed",
			"event", "api_call",
			"operation", operation,
			"service", service,
			"duration", duration,
			"success", success,
		)
	} else {
		l.Info("API call completed",
			"event", "api_call",
			"operation", operation,
			"service", service,
			"duration", duration,
			"success", success,
		)
	}
}

// ResourceUtilization logs resource utilization metrics
func (l *Logger) ResourceUtilization(nodeName string, cpuUsage, memoryUsage float64) {
	l.V(1).Info("Resource utilization",
		"event", "resource_utilization",
		"nodeName", nodeName,
		"cpuUsage", fmt.Sprintf("%.2f%%", cpuUsage),
		"memoryUsage", fmt.Sprintf("%.2f%%", memoryUsage),
	)
}

// Scaling logs scaling events
func (l *Logger) Scaling(direction string, nodeCount int, reason string) {
	l.Info("Scaling event",
		"event", "scaling",
		"direction", direction,
		"nodeCount", nodeCount,
		"reason", reason,
	)
}

// ErrorWithDetails logs errors with additional context
func (l *Logger) ErrorWithDetails(err error, msg string, keysAndValues ...interface{}) {
	allValues := append([]interface{}{"error", err}, keysAndValues...)
	l.Error(err, msg, allValues...)
}

// InstanceLifecycle logs instance lifecycle events
func (l *Logger) InstanceLifecycle(instanceID, event, status string) {
	l.Info("Instance lifecycle event",
		"event", "instance_lifecycle",
		"instanceID", instanceID,
		"lifecycleEvent", event,
		"status", status,
	)
}
