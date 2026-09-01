package taskqueue

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	q := New(0)
	if q.Len() != 0 {
		t.Errorf("Len = %d, want 0", q.Len())
	}
	if q.Capacity() != DefaultCapacity {
		t.Errorf("Capacity = %d, want %d", q.Capacity(), DefaultCapacity)
	}
}

func TestSubmit(t *testing.T) {
	q := New(10)
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
	q := New(10)
	_, err := q.Submit("", nil, 0, nil, "", nil)
	if err == nil {
		t.Fatal("expected error for empty type")
	}
}

func TestSubmit_Idempotency(t *testing.T) {
	q := New(10)
	id1, _ := q.Submit("test", nil, 0, nil, "key-123", nil)
	id2, _ := q.Submit("test", nil, 0, nil, "key-123", nil)
	if id1 != id2 {
		t.Errorf("expected same ID for duplicate idempotency key, got %q vs %q", id1, id2)
	}
}

func TestSubmit_IdempotencyCompleted(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "done-key", nil)
	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkRunning(id); err != nil {
		t.Fatal(err)
	}
	if err := q.Complete(id); err != nil {
		t.Fatal(err)
	}
	id2, err := q.Submit("test", nil, 0, nil, "done-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("completed idempotency key should return same id: %q vs %q", id, id2)
	}
}

func TestSubmit_IdempotencyConcurrent(t *testing.T) {
	q := New(100)
	const workers = 32
	ids := make([]string, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			id, err := q.Submit("test", nil, 0, nil, "race-key", nil)
			if err != nil {
				t.Errorf("Submit: %v", err)
				return
			}
			ids[idx] = id
		}(i)
	}
	wg.Wait()
	first := ids[0]
	for _, id := range ids[1:] {
		if id != first {
			t.Fatalf("concurrent idempotency returned different ids: %q vs %q", first, id)
		}
	}
	if q.Len() != 1 {
		t.Fatalf("Len = %d, want 1", q.Len())
	}
}

func TestSubmit_QueueFull(t *testing.T) {
	q := New(2)
	if _, err := q.Submit("a", nil, 0, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Submit("b", nil, 0, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	_, err := q.Submit("c", nil, 0, nil, "", nil)
	if err == nil {
		t.Fatal("expected error when queue is full")
	}
}

func TestGet_CopyOnRead(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", []byte("orig"), 0, nil, "", nil)
	task, _ := q.Get(id)
	task.Status = StatusCompleted
	task.Payload[0] = 'X'

	live, _ := q.Get(id)
	if live.Status != StatusPending {
		t.Fatalf("caller mutation leaked: status=%q", live.Status)
	}
	if live.Payload[0] != 'o' {
		t.Fatalf("caller mutation leaked payload")
	}
}

func TestList_CopyOnRead(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "", nil)
	tasks := q.List("", "")
	tasks[0].Status = StatusCompleted

	live, _ := q.Get(id)
	if live.Status != StatusPending {
		t.Fatal("List copy-on-read failed")
	}
}

func TestCancel(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "", nil)

	if err := q.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	task, _ := q.Get(id)
	if task.Status != StatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, StatusCancelled)
	}

	if err := q.Cancel(id); err == nil {
		t.Fatal("expected error for double cancel")
	}
	if err := q.Cancel("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestAssignComplete(t *testing.T) {
	q := New(10)
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

	if err := q.Assign(id, "node-2"); err == nil {
		t.Fatal("expected error for assigning already-assigned task")
	}
}

func TestMarkRunning(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "", nil)
	if err := q.MarkRunning(id); err == nil {
		t.Fatal("expected error marking pending task running")
	}
	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkRunning(id); err != nil {
		t.Fatal(err)
	}
	task, _ := q.Get(id)
	if task.Status != StatusRunning {
		t.Fatalf("status=%q", task.Status)
	}
	if task.StartedAt.IsZero() {
		t.Fatal("StartedAt should be set")
	}
}

func TestFailWithRetry(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 3, nil, "", nil)

	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := q.MarkRunning(id); err != nil {
		t.Fatal(err)
	}
	if err := q.Fail(id, "transient error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

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
	q := New(10)
	id, _ := q.Submit("test", nil, 1, nil, "", nil)

	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := q.Fail(id, "permanent error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	task, _ := q.Get(id)
	if task.Status != StatusFailed {
		t.Errorf("Status = %q, want %q", task.Status, StatusFailed)
	}
}

func TestFail_RejectsPending(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 1, nil, "", nil)
	if err := q.Fail(id, "nope"); err == nil {
		t.Fatal("expected error failing pending task")
	}
}

func TestComplete(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "", nil)

	if err := q.Complete(id); err == nil {
		t.Fatal("expected error completing pending task")
	}
	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := q.Complete(id); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	task, _ := q.Get(id)
	if task.Status != StatusCompleted {
		t.Errorf("Status = %q, want %q", task.Status, StatusCompleted)
	}
}

func TestComplete_RejectsCancelled(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "", nil)
	if err := q.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if err := q.Complete(id); err == nil {
		t.Fatal("expected error completing cancelled task")
	}
}

func TestComplete_RejectsAlreadyCompleted(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "", nil)
	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatal(err)
	}
	if err := q.Complete(id); err != nil {
		t.Fatal(err)
	}
	if err := q.Complete(id); err == nil {
		t.Fatal("expected error for double complete")
	}
}

func TestList(t *testing.T) {
	q := New(10)
	if _, err := q.Submit("type-a", nil, 0, nil, "", nil); err != nil {
		t.Fatalf("Submit type-a: %v", err)
	}
	if _, err := q.Submit("type-b", nil, 0, nil, "", nil); err != nil {
		t.Fatalf("Submit type-b: %v", err)
	}
	if _, err := q.Submit("type-a", nil, 0, nil, "", nil); err != nil {
		t.Fatalf("Submit type-a: %v", err)
	}

	if len(q.List("", "")) != 3 {
		t.Errorf("List() = %d, want 3", len(q.List("", "")))
	}
	if len(q.List("", "type-a")) != 2 {
		t.Errorf("List(type-a) = %d, want 2", len(q.List("", "type-a")))
	}
}

func TestPendingTasks(t *testing.T) {
	q := New(10)
	if _, err := q.Submit("a", nil, 0, nil, "", nil); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	id2, _ := q.Submit("b", nil, 0, nil, "", nil)
	if err := q.Assign(id2, "node-1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	pending := q.PendingTasks()
	if len(pending) != 1 {
		t.Errorf("PendingTasks = %d, want 1", len(pending))
	}
}

func TestReassign(t *testing.T) {
	q := New(10)
	id, _ := q.Submit("test", nil, 0, nil, "", nil)
	if err := q.Assign(id, "node-1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := q.Complete(id); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if err := q.Reassign(id); err == nil {
		t.Fatal("expected error for reassigning completed task")
	}

	id2, _ := q.Submit("test", nil, 0, nil, "", nil)
	if err := q.Assign(id2, "node-2"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := q.Reassign(id2); err != nil {
		t.Fatalf("Reassign: %v", err)
	}
	task, _ := q.Get(id2)
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want %q after reassign", task.Status, StatusPending)
	}
}
