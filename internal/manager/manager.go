package manager

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"zipget/internal/config"
	"zipget/internal/logger"
	"zipget/internal/model"
)

type (
	Task = model.Task
	File = model.File
)

type Loader interface {
	Check(ctx context.Context, urls []string) ([]File, error)
	Download(ctx context.Context, urls []string, out io.Writer) ([]File, error)
}

type Storage interface {
	CreateTask(ctx context.Context) (int64, error)
	DeleteTask(ctx context.Context, taskID int64) error
	AddFileToTask(ctx context.Context, taskID int64, url string) error
	GetTaskFiles(taskID int64) ([]File, error)
	UpdateTaskFiles(taskID int64, files []File) (Task, error)
}

var (
	ErrTaskNotFound     = model.ErrTaskNotFound
	ErrMaxFilesExceeded = model.ErrMaxFilesExceeded
	ErrServerBusy       = model.ErrServerBusy
	ErrServerCancelled  = model.ErrServerCancelled
)

type Manager struct {
	cfg      config.Manager
	stor     Storage
	loader   Loader
	muActive sync.Mutex
	active   int // количество активных загрузок
}

func New(cfg config.Manager, stor Storage, ldr Loader) *Manager {
	slog.Debug("new manager", "cfg", cfg)
	m := &Manager{
		cfg:    cfg,
		stor:   stor,
		loader: ldr,
	}
	return m
}

func (m *Manager) CreateTask(ctx context.Context) (int64, error) {
	return m.stor.CreateTask(ctx)
}

func (m *Manager) DeleteTask(ctx context.Context, taskID int64) error {
	return m.stor.DeleteTask(ctx, taskID)
}

func (m *Manager) AddFileToTask(ctx context.Context, taskID int64, url string) error {
	return m.stor.AddFileToTask(ctx, taskID, url)
}

func (m *Manager) GetTaskStatus(ctx context.Context, taskID int64) (Task, error) {
	files, err := m.stor.GetTaskFiles(taskID)
	if err != nil {
		return Task{}, err
	}

	// составляем список URLs требующих проверки (еще не проверяли или BadGateway на прошлой проверке)
	urls := make([]string, 0, len(files))
	ids := make([]int64, 0, len(files))

	// Запоминае ID
	for i := range files {
		if s := files[i].Status; s == 0 || s == http.StatusBadGateway {
			urls = append(urls, files[i].URL)
			ids = append(ids, files[i].ID)
		}
	}

	// чекаем URLs
	if len(urls) > 0 {
		files, err = m.loader.Check(ctx, urls)
		if err != nil {
			return Task{}, err
		}
	}

	// Востанавливаем ID
	for i, id := range ids {
		files[i].ID = id
	}

	return m.stor.UpdateTaskFiles(taskID, files)
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

	// ТОЛЬКО ДЛЯ ТЕСТОВ создаем задержку, чтобы можно было отследить активные задачи
	if m.cfg.ProcessDelay > 0 {
		logger.FromContext(ctx).Debug("process delay", "delay", m.cfg.ProcessDelay.String())
		time.Sleep(m.cfg.ProcessDelay)
	}

	files, err := m.stor.GetTaskFiles(taskID)
	if err != nil {
		return err
	}

	// составляем список URLs для загрузки (еще не проверяли или OK на прошлой проверке)
	urls := make([]string, 0, len(files))
	ids := make([]int64, 0, len(files))

	// Запоминаем ID
	for i := range files {
		if s := files[i].Status; s == 0 || s == http.StatusOK {
			urls = append(urls, files[i].URL)
			ids = append(ids, files[i].ID)
		}
	}

	// загружаем
	files, err = m.loader.Download(ctx, urls, out)
	if err != nil {
		return err
	}

	// Востанавливаем ID
	for i, id := range ids {
		files[i].ID = id
	}

	// игнорируем возвращаемые значения (мы свою работу *по загрузке* сделали)
	_, _ = m.stor.UpdateTaskFiles(taskID, files)

	return nil
}
