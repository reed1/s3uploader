package client

import (
	"sync"
)

type QueueEntry struct {
	LocalPath    string
	RemotePath   string
	AttemptCount int
}

type Queue struct {
	mu      sync.Mutex
	entries []QueueEntry
	set     map[string]struct{}
}

func NewQueue() *Queue {
	return &Queue{
		entries: make([]QueueEntry, 0),
		set:     make(map[string]struct{}),
	}
}

func (q *Queue) Enqueue(localPath, remotePath string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.set[localPath]; exists {
		return false
	}

	q.entries = append(q.entries, QueueEntry{
		LocalPath:  localPath,
		RemotePath: remotePath,
	})
	q.set[localPath] = struct{}{}
	return true
}

func (q *Queue) EnqueueWithAttempts(localPath, remotePath string, attempts int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.set[localPath]; exists {
		return false
	}

	q.entries = append(q.entries, QueueEntry{
		LocalPath:    localPath,
		RemotePath:   remotePath,
		AttemptCount: attempts,
	})
	q.set[localPath] = struct{}{}
	return true
}

func (q *Queue) Dequeue() (QueueEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) == 0 {
		return QueueEntry{}, false
	}

	entry := q.entries[0]
	q.entries = q.entries[1:]
	delete(q.set, entry.LocalPath)
	return entry, true
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

func (q *Queue) Contains(localPath string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, exists := q.set[localPath]
	return exists
}
