package manager

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"slices"
	"sync"
	"time"

	"zipget/internal/config"
	"zipget/internal/loader"
	"zipget/internal/model"
)

const (
	cleanTimeout = 1 * time.Minute
)

type (
	Task = model.Task
	File = model.File
)

var (
	ErrTaskNotFound     = model.ErrTaskNotFound
	ErrMaxFilesExceeded = model.ErrMaxFilesExceeded
	ErrServerBusy       = model.ErrServerBusy
	ErrServerCancelled  = model.ErrServerCancelled
)

type Manager struct {
	cfg       config.Manager
	loader    *loader.Loader
	mu        sync.RWMutex
	tasks     map[int64]*model.Task
	cancel    context.CancelFunc
	cancelled bool
	muActive  sync.Mutex
	active    int // количество активных загрузок
}

func New(cfg config.Manager, ldr *loader.Loader) *Manager {
	m := &Manager{
		cfg:    cfg,
		loader: ldr,
		tasks:  make(map[int64]*model.Task),
	}
	m.startTaskCleaner()
	return m
}

func (m *Manager) CreateTask(ctx context.Context) (int64, error) {
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

func (m *Manager) DeleteTask(ctx context.Context, taskID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelled {
		return ErrServerCancelled
	}

	// не проверяем наличие задачи для обеспечения идемпотентности
	delete(m.tasks, taskID)
	return nil
}

func (m *Manager) AddFileToTask(ctx context.Context, taskID int64, url string) error {
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

func (m *Manager) getTaskFiles(taskID int64) ([]File, error) {
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

func (m *Manager) updateTaskFiles(taskID int64, idxs []int, files []File) (Task, error) {
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

func (m *Manager) GetTaskStatus(ctx context.Context, taskID int64) (Task, error) {
	files, err := m.getTaskFiles(taskID)
	if err != nil {
		return Task{}, err
	}

	// составляем список URLs требующих проверки (еще не проверяли или BadGateway на прошлой проверке)
	urls := make([]string, 0, len(files))
	idxs := make([]int, 0, len(files))

	for i := range files {
		if s := files[i].Status; s == 0 || s == http.StatusBadGateway {
			urls = append(urls, files[i].URL)
			idxs = append(idxs, i)
		}
	}

	// чекаем URLs
	if len(urls) > 0 {
		files, err = m.loader.Check(ctx, urls)
		if err != nil {
			return Task{}, err
		}
	}

	return m.updateTaskFiles(taskID, idxs, files)
}

func (m *Manager) getDownloadSlot() bool {
	m.muActive.Lock()
	defer m.muActive.Unlock()

	if m.active < m.cfg.MaxActive {
		m.active++
		return true
	}

	return false
}

func (m *Manager) freeDownloadSlot() {
	m.muActive.Lock()
	defer m.muActive.Unlock()
	m.active--
}

func (m *Manager) ProcessTask(ctx context.Context, taskID int64, out io.Writer) error {
	if !m.getDownloadSlot() {
		return ErrServerBusy
	}
	defer m.freeDownloadSlot()

	files, err := m.getTaskFiles(taskID)
	if err != nil {
		return err
	}

	// составляем список URLs для загрузки (еще не проверяли или OK на прошлой проверке)
	urls := make([]string, 0, len(files))
	idxs := make([]int, 0, len(files))

	for i := range files {
		if s := files[i].Status; s == 0 || s == http.StatusOK {
			urls = append(urls, files[i].URL)
			idxs = append(idxs, i)
		}
	}

	// загружаем
	files, err = m.loader.Download(ctx, urls, out)
	if err != nil {
		return err
	}

	// игнорируем возвращаемые значения (мы свою работу *по загрузке* сделали)
	_, _ = m.updateTaskFiles(taskID, idxs, files)

	return nil
}

func (m *Manager) cleanExpiredTasks() {
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

func (m *Manager) startTaskCleaner() {
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

func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cancelled {
		m.cancel()
		clear(m.tasks)
		m.cancelled = true
	}
}
