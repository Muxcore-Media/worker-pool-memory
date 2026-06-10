package taskqueue

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// TaskStatus represents the lifecycle of a task.
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusAssigned  TaskStatus = "assigned"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusCancelled TaskStatus = "cancelled"
)

// Task is a unit of work.
type Task struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Payload       []byte            `json:"payload,omitempty"`
	AssignedNode  string            `json:"assigned_node,omitempty"`
	Status        TaskStatus        `json:"status"`
	MaxRetries    int               `json:"max_retries"`
	RetryCount    int               `json:"retry_count"`
	Capabilities  []string          `json:"capabilities,omitempty"`
	Error         string            `json:"error,omitempty"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"`
	Meta          map[string]any    `json:"meta,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	StartedAt     time.Time         `json:"started_at,omitempty"`
	CompletedAt   time.Time         `json:"completed_at,omitempty"`
}

// Queue is an in-memory goroutine-safe task store.
type Queue struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

// New creates an empty task queue.
func New() *Queue {
	return &Queue{tasks: make(map[string]*Task)}
}

// Submit creates a new task in pending state. Returns the task ID.
func (q *Queue) Submit(taskType string, payload []byte, maxRetries int, capabilities []string, idempotencyKey string, meta map[string]any) (string, error) {
	if taskType == "" {
		return "", fmt.Errorf("task type is required")
	}

	// Check idempotency key for duplicates.
	if idempotencyKey != "" {
		q.mu.RLock()
		for _, t := range q.tasks {
			if t.IdempotencyKey == idempotencyKey && t.Status != StatusFailed {
				q.mu.RUnlock()
				return t.ID, nil
			}
		}
		q.mu.RUnlock()
	}

	task := &Task{
		ID:             newID(),
		Type:           taskType,
		Payload:        payload,
		Status:         StatusPending,
		MaxRetries:     maxRetries,
		Capabilities:   capabilities,
		IdempotencyKey: idempotencyKey,
		Meta:           meta,
		CreatedAt:      time.Now(),
	}

	q.mu.Lock()
	q.tasks[task.ID] = task
	q.mu.Unlock()
	return task.ID, nil
}

// Get returns a task by ID.
func (q *Queue) Get(id string) (*Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	t, ok := q.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	return t, nil
}

// Cancel sets a pending/assigned task to cancelled. Returns error for already
// running or completed tasks.
func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	switch t.Status {
	case StatusPending, StatusAssigned:
		t.Status = StatusCancelled
		return nil
	case StatusRunning:
		return fmt.Errorf("task %q is currently running, cannot cancel", id)
	case StatusCompleted:
		return fmt.Errorf("task %q is already completed", id)
	case StatusCancelled:
		return fmt.Errorf("task %q is already cancelled", id)
	default:
		return fmt.Errorf("task %q has status %q, cannot cancel", id, t.Status)
	}
}

// Assign sets a task's status to assigned and records the node.
func (q *Queue) Assign(id, nodeID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	if t.Status != StatusPending {
		return fmt.Errorf("task %q is not pending (status: %s)", id, t.Status)
	}
	t.Status = StatusAssigned
	t.AssignedNode = nodeID
	return nil
}

// Complete sets a task to completed.
func (q *Queue) Complete(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	t.Status = StatusCompleted
	t.CompletedAt = time.Now()
	return nil
}

// Fail sets a task to failed. If retries remain, resets to pending.
func (q *Queue) Fail(id, errMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	t.RetryCount++
	t.Error = errMsg
	if t.RetryCount < t.MaxRetries {
		t.Status = StatusPending
		t.AssignedNode = ""
		return nil
	}
	t.Status = StatusFailed
	t.CompletedAt = time.Now()
	return nil
}

// List returns tasks filtered by status and/or type.
func (q *Queue) List(statusFilter, typeFilter string) []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*Task, 0, len(q.tasks))
	for _, t := range q.tasks {
		if statusFilter != "" && string(t.Status) != statusFilter {
			continue
		}
		if typeFilter != "" && t.Type != typeFilter {
			continue
		}
		result = append(result, t)
	}
	return result
}

// PendingTasks returns all pending tasks sorted by creation time (oldest first).
func (q *Queue) PendingTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var result []*Task
	for _, t := range q.tasks {
		if t.Status == StatusPending {
			result = append(result, t)
		}
	}
	return result
}

// Reassign sets a task back to pending for reassignment.
func (q *Queue) Reassign(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	if t.Status == StatusCompleted || t.Status == StatusCancelled {
		return fmt.Errorf("task %q cannot be reassigned (status: %s)", id, t.Status)
	}
	t.Status = StatusPending
	t.AssignedNode = ""
	return nil
}

// Len returns the total number of tasks.
func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tasks)
}

// Stats returns count of tasks by status.
func (q *Queue) Stats() map[string]int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := map[string]int{
		"pending": 0, "assigned": 0, "running": 0,
		"completed": 0, "failed": 0, "cancelled": 0,
	}
	for _, t := range q.tasks {
		stats[string(t.Status)]++
	}
	return stats
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
