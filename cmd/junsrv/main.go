package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Конфигурация сервера
const (
	maxActiveTasks  = 3       // Максимум одновременно обрабатываемых задач
	maxFilesPerTask = 3       // Максимум файлов на одну задачу
	serverPort      = ":8080" // Порт сервера
)

var (
	tasks     = make(map[int]*Task)                 // Хранилище задач
	taskMutex sync.RWMutex                          // Мьютекс для безопасного доступа к tasks
	nextID    = 1                                   // Счетчик для ID задач
	activeSem = make(chan struct{}, maxActiveTasks) // Семафор для ограничения активных задач
)

// Ошибки
var (
	ErrTooManyTasks    = errors.New("server is busy, try again later")
	ErrInvalidFileType = errors.New("only PDF and JPEG files are allowed")
	ErrMaxFilesReached = errors.New("maximum files per task reached")
)

type Task struct {
	ID        int       `json:"id"`
	Files     []File    `json:"files"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	ZipPath   string    `json:"zip_path,omitempty"`
	fileMutex sync.Mutex
}

type File struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Error    string `json:"error,omitempty"`
	FileType string `json:"file_type,omitempty"`
}

func main() {
	// Создаем папку для архивов
	if err := os.Mkdir("zips", 0755); err != nil && !os.IsExist(err) {
		log.Fatal("Failed to create zips directory:", err)
	}

	// Настраиваем обработчики
	http.HandleFunc("/api/tasks", tasksHandler)
	http.HandleFunc("/api/tasks/", taskHandler)
	http.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir("./zips"))))

	log.Println("Server started on", serverPort)
	log.Fatal(http.ListenAndServe(serverPort, nil))
}

// Обработчик для /api/tasks
func tasksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	select {
	case activeSem <- struct{}{}: // Занимаем слот
	default:
		http.Error(w, ErrTooManyTasks.Error(), http.StatusTooManyRequests)
		return
	}

	taskMutex.Lock()
	id := nextID
	nextID++
	task := &Task{
		ID:        id,
		CreatedAt: time.Now(),
	}
	tasks[id] = task
	taskMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"task_id": id})
}

// Обработчик для /api/tasks/{id}
func taskHandler(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(pathParts[3])
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	taskMutex.RLock()
	task, exists := tasks[id]
	taskMutex.RUnlock()

	if !exists {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	switch {
	case r.Method == http.MethodGet:
		handleGetTask(w, task)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/files"):
		handleAddFile(w, r, task)
	case r.Method == http.MethodDelete:
		handleDeleteTask(w, task)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAddFile(w http.ResponseWriter, r *http.Request, task *Task) {
	task.fileMutex.Lock()
	defer task.fileMutex.Unlock()

	if len(task.Files) >= maxFilesPerTask {
		http.Error(w, ErrMaxFilesReached.Error(), http.StatusForbidden)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	file := File{URL: req.URL}

	// Проверяем доступность и тип файла
	resp, err := http.Head(req.URL)
	if err != nil {
		file.Status = http.StatusBadGateway
		file.Error = "Failed to access file"
	} else {
		defer resp.Body.Close()
		file.Status = resp.StatusCode

		if resp.StatusCode == http.StatusOK {
			contentType := resp.Header.Get("Content-Type")
			switch {
			case strings.Contains(contentType, "application/pdf"):
				file.FileType = "pdf"
			case strings.Contains(contentType, "image/jpeg"):
				file.FileType = "jpg"
			default:
				file.Status = http.StatusUnsupportedMediaType
				file.Error = ErrInvalidFileType.Error()
			}
		}
	}

	task.Files = append(task.Files, file)
	task.UpdatedAt = time.Now()
	w.WriteHeader(http.StatusOK)
}

func handleGetTask(w http.ResponseWriter, task *Task) {
	task.fileMutex.Lock()
	defer task.fileMutex.Unlock()

	// Создаем архив при достижении лимита файлов
	if len(task.Files) >= maxFilesPerTask && task.ZipPath == "" {
		if err := createZip(task); err == nil {
			task.ZipPath = "task_" + strconv.Itoa(task.ID) + ".zip"
			task.UpdatedAt = time.Now()

			// Освобождаем слот
			<-activeSem
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func handleDeleteTask(w http.ResponseWriter, task *Task) {
	taskMutex.Lock()
	delete(tasks, task.ID)
	taskMutex.Unlock()

	// Удаляем архив
	if task.ZipPath != "" {
		os.Remove("./zips/" + task.ZipPath)
	}

	w.WriteHeader(http.StatusOK)
}

func createZip(task *Task) error {
	zipPath := "zips/task_" + strconv.Itoa(task.ID) + ".zip"
	zf, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zf.Close()

	zw := zip.NewWriter(zf)
	defer zw.Close()

	for _, file := range task.Files {
		// Пропускаем недоступные или неподходящие файлы
		if file.Status != http.StatusOK || file.FileType == "" {
			continue
		}

		resp, err := http.Get(file.URL)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		// Формируем имя файла
		rawName := file.URL[strings.LastIndex(file.URL, "/")+1:]
		baseName := strings.SplitN(rawName, "?", 2)[0]
		ext := "." + file.FileType

		// Убедимся, что имя файла имеет правильное расширение
		if !strings.HasSuffix(strings.ToLower(baseName), ext) {
			if dot := strings.LastIndex(baseName, "."); dot != -1 {
				baseName = baseName[:dot]
			}
			baseName += ext
		}

		if baseName == ext {
			baseName = "file_" + strconv.Itoa(len(task.Files)) + ext
		}

		f, err := zw.Create(baseName)
		if err != nil {
			continue
		}
		io.Copy(f, resp.Body)
	}

	return nil
}
