package memstor

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"zipget/internal/model"
)

const (
	cleanTimeout = 1 * time.Minute
)

type (
	Task = model.Task
	File = model.File
)

type Config struct {
	MaxTotal int
	MaxFiles int
	TaskTTL  time.Duration
}

var (
	ErrTaskNotFound     = model.ErrTaskNotFound
	ErrMaxFilesExceeded = model.ErrMaxFilesExceeded
	ErrServerBusy       = model.ErrServerBusy
	ErrServerCancelled  = model.ErrServerCancelled
)

type Memstor struct {
	cfg       Config
	mu        sync.RWMutex
	tasks     map[int64]*model.Task
	cancel    context.CancelFunc
	cancelled bool
}

func New(cfg Config) *Memstor {
	m := &Memstor{
		cfg:   cfg,
		tasks: make(map[int64]*model.Task),
	}
	m.startTaskCleaner()
	return m
}

func (m *Memstor) CreateTask(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelled {
		return 0, ErrServerCancelled
	}

	if m.cfg.MaxTotal >= 0 && len(m.tasks) >= m.cfg.MaxTotal { // если m.cfg.MaxTotal < 0, то неограничено, если 0 - запрешено
		return 0, ErrServerBusy
	}

	id := rand.Int64()
	m.tasks[id] = &model.Task{
		ID:        id,
		Files:     make([]model.File, 0),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(m.cfg.TaskTTL),
	}

	return id, nil
}

func (m *Memstor) DeleteTask(ctx context.Context, taskID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelled {
		return ErrServerCancelled
	}

	// не проверяем наличие задачи для обеспечения идемпотентности
	delete(m.tasks, taskID)
	return nil
}

func (m *Memstor) AddFileToTask(ctx context.Context, taskID int64, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelled {
		return ErrServerCancelled
	}

	task, exists := m.tasks[taskID]
	if !exists {
		return ErrTaskNotFound
	}

	if m.cfg.MaxFiles >= 0 && len(task.Files) >= m.cfg.MaxFiles { // если m.cfg.MaxFiles < 0, то неограничено, если 0 - запрешено
		return ErrMaxFilesExceeded
	}

	task.Files = append(task.Files, File{URL: url})
	return nil
}

func (m *Memstor) GetTaskFiles(taskID int64) ([]File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.cancelled {
		return nil, ErrServerCancelled
	}

	task, exists := m.tasks[taskID]
	if !exists {
		return nil, ErrTaskNotFound
	}

	return slices.Clone(task.Files), nil
}

func (m *Memstor) UpdateTaskFiles(taskID int64, idxs []int, files []File) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelled {
		return Task{}, ErrServerCancelled
	}

	task, exists := m.tasks[taskID]
	if !exists {
		return Task{}, ErrTaskNotFound
	}

	if len(idxs) > 0 {
		for i, idx := range idxs {
			task.Files[idx] = files[i]
		}
		task.UpdatedAt = time.Now()
	}

	return task.Clone(), nil
}

func (m *Memstor) cleanExpiredTasks() {
	// FIXME: для перформанса нужно использовать PriorityQueue по ExpiresAt

	var expiredTasks []int64
	func() {
		m.mu.RLock()
		defer m.mu.RUnlock()

		now := time.Now()
		for _, task := range m.tasks {
			if task.ExpiresAt.Before(now) {
				expiredTasks = append(expiredTasks, task.ID)
			}
		}
	}()

	if len(expiredTasks) > 0 {
		m.mu.Lock()
		defer m.mu.Unlock()

		for _, taskID := range expiredTasks {
			delete(m.tasks, taskID)
		}
	}
}

func (m *Memstor) startTaskCleaner() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	go func() {
		tm := time.NewTimer(cleanTimeout)
		defer tm.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-tm.C:
				m.cleanExpiredTasks()
				tm.Reset(cleanTimeout)
			}
		}
	}()
}

func (m *Memstor) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cancelled {
		m.cancel()
		clear(m.tasks)
		m.cancelled = true
	}
}
