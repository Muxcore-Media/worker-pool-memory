package taskqueue

import (
	"testing"
)

func TestNew(t *testing.T) {
	q := New()
	if q.Len() != 0 {
		t.Errorf("Len = %d, want 0", q.Len())
	}
}

func TestSubmit(t *testing.T) {
	q := New()
	id, err := q.Submit("test", []byte("data"), 3, []string{"worker"}, "", nil)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	if q.Len() != 1 {
		t.Errorf("Len = %d, want 1", q.Len())
	}

	task, err := q.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Type != "test" {
		t.Errorf("Type = %q, want %q", task.Type, "test")
	}
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want %q", task.Status, StatusPending)
	}
}

func TestSubmit_EmptyType(t *testing.T) {
	q := New()
	_, err := q.Submit("", nil, 0, nil, "", nil)
	if err == nil {
		t.Fatal("expected error for empty type")
	}
}

func TestSubmit_Idempotency(t *testing.T) {
	q := New()
	id1, _ := q.Submit("test", nil, 0, nil, "key-123", nil)
	id2, _ := q.Submit("test", nil, 0, nil, "key-123", nil)
	if id1 != id2 {
		t.Errorf("expected same ID for duplicate idempotency key, got %q vs %q", id1, id2)
	}
}

func TestCancel(t *testing.T) {
	q := New()
	id, _ := q.Submit("test", nil, 0, nil, "", nil)

	if err := q.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	task, _ := q.Get(id)
	if task.Status != StatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, StatusCancelled)
	}

	// Cancelling again should fail.
	if err := q.Cancel(id); err == nil {
		t.Fatal("expected error for double cancel")
	}

	// Cancelling nonexistent should fail.
	if err := q.Cancel("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestAssignComplete(t *testing.T) {
	q := New()
	id, _ := q.Submit("test", nil, 0, nil, "", nil)

	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	task, _ := q.Get(id)
	if task.Status != StatusAssigned {
		t.Errorf("Status = %q, want %q", task.Status, StatusAssigned)
	}
	if task.AssignedNode != "node-1" {
		t.Errorf("AssignedNode = %q, want %q", task.AssignedNode, "node-1")
	}

	// Assigning again should fail.
	if err := q.Assign(id, "node-2"); err == nil {
		t.Fatal("expected error for assigning already-assigned task")
	}
}

func TestFailWithRetry(t *testing.T) {
	q := New()
	id, _ := q.Submit("test", nil, 3, nil, "", nil)

	q.Assign(id, "node-1")
	q.Fail(id, "transient error")

	task, _ := q.Get(id)
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want %q (should reset to pending for retry)", task.Status, StatusPending)
	}
	if task.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", task.RetryCount)
	}
	if task.AssignedNode != "" {
		t.Errorf("AssignedNode should be cleared for retry, got %q", task.AssignedNode)
	}
}

func TestFailNoRetry(t *testing.T) {
	q := New()
	id, _ := q.Submit("test", nil, 1, nil, "", nil)

	q.Assign(id, "node-1")
	q.Fail(id, "permanent error")

	task, _ := q.Get(id)
	if task.Status != StatusFailed {
		t.Errorf("Status = %q, want %q", task.Status, StatusFailed)
	}
}

func TestComplete(t *testing.T) {
	q := New()
	id, _ := q.Submit("test", nil, 0, nil, "", nil)

	q.Assign(id, "node-1")
	q.Complete(id)

	task, _ := q.Get(id)
	if task.Status != StatusCompleted {
		t.Errorf("Status = %q, want %q", task.Status, StatusCompleted)
	}
}

func TestList(t *testing.T) {
	q := New()
	q.Submit("type-a", nil, 0, nil, "", nil)
	q.Submit("type-b", nil, 0, nil, "", nil)
	q.Submit("type-a", nil, 0, nil, "", nil)

	if len(q.List("", "")) != 3 {
		t.Errorf("List() = %d, want 3", len(q.List("", "")))
	}
	if len(q.List("", "type-a")) != 2 {
		t.Errorf("List(type-a) = %d, want 2", len(q.List("", "type-a")))
	}
}

func TestPendingTasks(t *testing.T) {
	q := New()
	q.Submit("a", nil, 0, nil, "", nil)
	id2, _ := q.Submit("b", nil, 0, nil, "", nil)
	q.Assign(id2, "node-1")

	pending := q.PendingTasks()
	if len(pending) != 1 {
		t.Errorf("PendingTasks = %d, want 1", len(pending))
	}
}

func TestReassign(t *testing.T) {
	q := New()
	id, _ := q.Submit("test", nil, 0, nil, "", nil)
	q.Assign(id, "node-1")
	q.Complete(id)

	if err := q.Reassign(id); err == nil {
		t.Fatal("expected error for reassigning completed task")
	}

	id2, _ := q.Submit("test", nil, 0, nil, "", nil)
	q.Assign(id2, "node-2")
	if err := q.Reassign(id2); err != nil {
		t.Fatalf("Reassign: %v", err)
	}
	task, _ := q.Get(id2)
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want %q after reassign", task.Status, StatusPending)
	}
}
