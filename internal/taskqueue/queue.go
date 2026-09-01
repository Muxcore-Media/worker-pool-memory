package taskqueue

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"
)

const DefaultCapacity = 10000

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
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Payload        []byte         `json:"payload,omitempty"`
	AssignedNode   string         `json:"assigned_node,omitempty"`
	Status         TaskStatus     `json:"status"`
	MaxRetries     int            `json:"max_retries"`
	RetryCount     int            `json:"retry_count"`
	Capabilities   []string       `json:"capabilities,omitempty"`
	Error          string         `json:"error,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Meta           map[string]any `json:"meta,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	StartedAt      time.Time      `json:"started_at,omitempty"`
	CompletedAt    time.Time      `json:"completed_at,omitempty"`
}

// Queue is an in-memory goroutine-safe task store.
type Queue struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	capacity int
}

// New creates an empty task queue with the given capacity (0 uses DefaultCapacity).
func New(capacity int) *Queue {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Queue{tasks: make(map[string]*Task), capacity: capacity}
}

// Submit creates a new task in pending state. Returns the task ID.
func (q *Queue) Submit(taskType string, payload []byte, maxRetries int, capabilities []string, idempotencyKey string, meta map[string]any) (string, error) {
	if taskType == "" {
		return "", fmt.Errorf("task type is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if idempotencyKey != "" {
		for _, t := range q.tasks {
			if t.IdempotencyKey == idempotencyKey && t.Status != StatusFailed {
				return t.ID, nil
			}
		}
	}

	if len(q.tasks) >= q.capacity {
		return "", fmt.Errorf("queue full (capacity %d)", q.capacity)
	}

	id, err := newID()
	if err != nil {
		return "", fmt.Errorf("generate task id: %w", err)
	}

	task := &Task{
		ID:             id,
		Type:           taskType,
		Payload:        append([]byte(nil), payload...),
		Status:         StatusPending,
		MaxRetries:     maxRetries,
		Capabilities:   append([]string(nil), capabilities...),
		IdempotencyKey: idempotencyKey,
		Meta:           copyMeta(meta),
		CreatedAt:      time.Now(),
	}

	q.tasks[task.ID] = task
	return task.ID, nil
}

// Get returns a copy of a task by ID.
func (q *Queue) Get(id string) (*Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	t, ok := q.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	return copyTask(t), nil
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

// MarkRunning marks an assigned task as running.
func (q *Queue) MarkRunning(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	switch t.Status {
	case StatusAssigned:
		t.Status = StatusRunning
		if t.StartedAt.IsZero() {
			t.StartedAt = time.Now()
		}
		return nil
	case StatusRunning:
		return nil
	default:
		return fmt.Errorf("task %q cannot start (status: %s)", id, t.Status)
	}
}

// Complete sets a task to completed.
func (q *Queue) Complete(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	switch t.Status {
	case StatusAssigned, StatusRunning:
		t.Status = StatusCompleted
		t.CompletedAt = time.Now()
		return nil
	case StatusCompleted:
		return fmt.Errorf("task %q is already completed", id)
	case StatusCancelled:
		return fmt.Errorf("task %q is cancelled", id)
	default:
		return fmt.Errorf("task %q cannot complete (status: %s)", id, t.Status)
	}
}

// Fail sets a task to failed. If retries remain, resets to pending.
func (q *Queue) Fail(id, errMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	switch t.Status {
	case StatusAssigned, StatusRunning:
	default:
		return fmt.Errorf("task %q cannot fail (status: %s)", id, t.Status)
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

// List returns copies of tasks filtered by status and/or type.
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
		result = append(result, copyTask(t))
	}
	return result
}

// PendingTasks returns copies of pending tasks.
func (q *Queue) PendingTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var result []*Task
	for _, t := range q.tasks {
		if t.Status == StatusPending {
			result = append(result, copyTask(t))
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

// Capacity returns the configured maximum number of tasks.
func (q *Queue) Capacity() int {
	return q.capacity
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

func copyTask(t *Task) *Task {
	if t == nil {
		return nil
	}
	out := *t
	if t.Payload != nil {
		out.Payload = append([]byte(nil), t.Payload...)
	}
	if t.Capabilities != nil {
		out.Capabilities = append([]string(nil), t.Capabilities...)
	}
	out.Meta = copyMeta(t.Meta)
	return &out
}

func copyMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return nil
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// ErrQueueFull is returned when Submit would exceed capacity.
var ErrQueueFull = errors.New("queue full")
