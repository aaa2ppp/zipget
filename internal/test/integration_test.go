// integration_test.go
package main

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"zipget/internal/test/files"
)

// Константы для конфигурации
const (
	testPort       = "8081"                  // Порт для тестов (отличается от основного 8080)
	testServerURL  = "http://localhost:8081" // URL тестового сервера
	fileServerPort = "8082"                  // Порт локального файл-сервера
	httpbinBaseURL = "https://httpbin.org"
)

// testEnv - переменные окружения, которые будут использоваться для тестового сервера.
// Они переопределяют значения из .env файла.
var testEnv = map[string]string{
	"LOG_LEVEL":             cmp.Or(os.Getenv("LOG_LEVEL"), "INFO"),
	"SERVER_ADDR":           ":" + testPort,
	"MANAGER_MAX_TOTAL":     "100",
	"MANAGER_MAX_ACTIVE":    "3",
	"MANAGER_MAX_FILES":     "3",
	"MANAGER_TASK_TTL":      "1m",
	"MANAGER_PROCESS_DELAY": "100ms",
	"LOADER_ALLOW_MIME":     "application/pdf image/jpeg", // Строго по ТЗ
}

var (
	testWorkDir string // рабочий каталог в котором будет запущен тест (должен быть абсолютным)
	binFile     string // путь к бинарнику сервера (может быть относительным от testWorkDir)
)

func init() {
	testWorkDir = os.Getenv("WORK_DIR")
	if testWorkDir == "" {
		log.Fatal("env WORK_DIR is required")
	}
	if err := os.MkdirAll(testWorkDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(testWorkDir); err != nil {
		log.Fatal(err)
	}

	binFile = os.Getenv("BIN_FILE")
	if binFile == "" {
		log.Fatal("env BIN_FILE is required")
	}
}

// Структуры для десериализации JSON-ответов
type CreateTaskResponse struct {
	TaskID int64 `json:"task_id"`
}

type GetTaskResponse struct {
	Task    Task   `json:"task"`
	Archive string `json:"archive,omitempty"`
}

type Task struct {
	ID        int64  `json:"id"`
	Files     []File `json:"files"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type File struct {
	URL         string `json:"url,omitempty"`
	OrigName    string `json:"orig_name,omitempty"`
	Name        string `json:"name,omitempty"`
	Status      int    `json:"status,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ErrorMsg    string `json:"error_msg,omitempty"`
}

// TestCreateTask проверяет создание новой задачи.
func TestCreateTask(t *testing.T) {
	resp, err := http.Post(testServerURL+"/api/tasks", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	var createResp CreateTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if createResp.TaskID <= 0 {
		t.Errorf("Expected positive task ID, got %d", createResp.TaskID)
	}
}

// TestDeleteTask проверяет удаление задачи и освобождение ресурсов.
func TestDeleteTask(t *testing.T) {
	taskID := createTask(t)

	// Удаляем задачу
	url := fmt.Sprintf("%s/api/tasks/%d", testServerURL, taskID)
	req, _ := http.NewRequest("DELETE", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for DELETE, got %d", resp.StatusCode)
	}

	// Пытаемся получить статус удаленной задачи
	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("Failed to send GET request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404 for deleted task, got %d", resp.StatusCode)
	}
}

// TestAddFileToTask проверяет добавление файла в задачу.
func TestAddFileToTask(t *testing.T) {
	// Сначала создаем задачу
	taskID := createTask(t)

	// Добавляем файл
	fileURL := httpbinBaseURL + "/bytes/1024"
	reqBody := fmt.Sprintf(`{"url": "%s"}`, fileURL)
	resp, err := http.Post(fmt.Sprintf("%s/api/tasks/%d/files", testServerURL, taskID), "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Проверяем, что файл добавлен
	taskResp := getTaskStatus(t, taskID)
	if len(taskResp.Task.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(taskResp.Task.Files))
	}
	if taskResp.Task.Files[0].URL != fileURL {
		t.Errorf("Expected URL %s, got %s", fileURL, taskResp.Task.Files[0].URL)
	}

	deleteTask(t, taskID)
}

// TestGetTaskStatus_ArchiveLink проверяет, что при 3 файлах возвращается ссылка на архив.
func TestGetTaskStatus_ArchiveLink(t *testing.T) {
	taskID := createTask(t)

	// Добавляем 3 файла
	urls := []string{
		httpbinBaseURL + "/bytes/1024",
		httpbinBaseURL + "/bytes/2048",
		httpbinBaseURL + "/bytes/4096",
	}
	for _, url := range urls {
		addFileToTask(t, taskID, url)
	}

	// Получаем статус
	taskResp := getTaskStatus(t, taskID)

	// Проверяем, что ссылка на архив присутствует
	if taskResp.Archive == "" {
		t.Error("Expected archive link to be present when task has 3 files")
	}
	if !strings.HasPrefix(taskResp.Archive, "/files/") {
		t.Errorf("Expected archive link to start with '/files/', got %s", taskResp.Archive)
	}

	deleteTask(t, taskID)
}

// TestAddFileToFullTask проверяет ограничение на 3 файла.
func TestAddFileToFullTask(t *testing.T) {
	taskID := createTask(t)

	// Заполняем задачу 3 файлами
	urls := []string{
		httpbinBaseURL + "/bytes/1024",
		httpbinBaseURL + "/bytes/2048",
		httpbinBaseURL + "/bytes/4096",
	}
	for _, url := range urls {
		addFileToTask(t, taskID, url)
	}

	// Пытаемся добавить 4-й файл
	fileURL := httpbinBaseURL + "/bytes/8192"
	reqBody := fmt.Sprintf(`{"url": "%s"}`, fileURL)
	resp, err := http.Post(testServerURL+fmt.Sprintf("/api/tasks/%d/files", taskID), "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	resp.Body.Close()

	// Ожидаем ошибку (409 Conflict или 400 Bad Request)
	if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 409 or 400 for full task, got %d", resp.StatusCode)
	}

	deleteTask(t, taskID)
}

// TestConcurrentArchiveGeneration проверяет, что число активных задач не превышает 3-х
func TestConcurrentArchiveGeneration(t *testing.T) {
	taskIDs := make([]int64, 4)
	for i := range taskIDs {
		taskIDs[i] = createTask(t)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	statuses := make(map[int64]int) // taskID -> status code
	errors := make([]string, 0)

	var client http.Client
	client.Timeout = 10 * time.Second

	for _, id := range taskIDs {
		wg.Add(1)
		go func(taskID int64) {
			defer wg.Done()

			resp, err := client.Get(fmt.Sprintf("%s/api/tasks/%d/archive", testServerURL, taskID))
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Sprintf("GET /api/tasks/%d/archive: %v", taskID, err))
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			_, _ = io.Copy(io.Discard, resp.Body)

			mu.Lock()
			statuses[taskID] = resp.StatusCode
			mu.Unlock()
		}(id)
	}

	wg.Wait()

	// Анализируем результаты
	if len(errors) > 0 {
		t.Fatalf("Request errors: %v", errors)
	}

	okCount := 0
	busyCount := 0
	for _, code := range statuses {
		switch code {
		case http.StatusOK:
			okCount++
		case http.StatusServiceUnavailable:
			busyCount++
		default:
			t.Errorf("Unexpected status code: %d", code)
		}
	}

	// Ожидаем: 3 успешных, 1 отклонён
	if okCount != 3 {
		t.Errorf("Expected 3 tasks to succeed, got %d", okCount)
	}
	if busyCount != 1 {
		t.Errorf("Expected 1 task to be rejected with 503, got %d", busyCount)
	}

	for _, id := range taskIDs {
		deleteTask(t, id)
	}
}

// TestUnavailableFile проверяет, что при недоступном файле остальные упаковываются.
func TestUnavailableFile(t *testing.T) {
	taskID := createTask(t)

	// Добавляем один доступный и один недоступный файл
	addFileToTask(t, taskID, httpbinBaseURL+"/image/jpeg") // 200 OK
	addFileToTask(t, taskID, httpbinBaseURL+"/bytes/1024") // 403 Запрещенный тип данных
	addFileToTask(t, taskID, httpbinBaseURL+"/status/404") // 404 Недоступный URL

	// Пытаемся получить архив (это запустит процесс загрузки)
	archiveURL := fmt.Sprintf("%s/api/tasks/%d/archive", testServerURL, taskID)
	resp, err := http.Get(archiveURL)
	if err != nil {
		t.Fatalf("Failed to download archive: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for archive download, got %d", resp.StatusCode)
	}

	// Проверяем, что ответ - это ZIP-архив
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/zip" {
		t.Errorf("Expected Content-Type 'application/zip', got '%s'", contentType)
	}

	// Проверяем, что архив не пустой
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("Downloaded archive is empty")
	}

	// Проверяем статус задачи
	// Ожидаем, что первый файл OK, остальные - ошибка
	taskResp := getTaskStatus(t, taskID)
	if len(taskResp.Task.Files) != 3 {
		t.Errorf("Expected 3 files in task, got %d", len(taskResp.Task.Files))
	}
	if taskResp.Task.Files[0].Status != http.StatusOK {
		t.Errorf("Expected first file status 200, got %d", taskResp.Task.Files[0].Status)
	}
	if taskResp.Task.Files[1].Status != http.StatusForbidden {
		t.Errorf("Expected second file status 403, got %d", taskResp.Task.Files[1].Status)
	}
	if taskResp.Task.Files[1].ErrorMsg == "" {
		t.Error("Expected error message for unavailable file")
	}
	if taskResp.Task.Files[2].Status != http.StatusNotFound {
		t.Errorf("Expected third file status 404, got %d", taskResp.Task.Files[2].Status)
	}
	if taskResp.Task.Files[2].ErrorMsg == "" {
		t.Error("Expected error message for unavailable file")
	}

	deleteTask(t, taskID)
}

// TestBlockPrivateURLs проверяет, что сервер запрещает загрузку файлов с приватных адресов
func TestBlockPrivateURLs(t *testing.T) {
	// Список URL, ведущих на приватные/локальные адреса
	privateURLs := []string{
		"http://localhost/robots.txt",
		"http://127.0.0.1/robots.txt",
		"http://192.168.0.1/status/200",
		"http://10.0.0.1/status/200",
		"http://172.16.0.1/status/200",
		"http://169.254.169.254/latest/meta-data/", // AWS metadata
		"http://[::1]/robots.txt",                  // IPv6 localhost
		"http://localhost:8080/status/200",         // с портом
	}

	for _, url := range privateURLs {
		t.Run(fmt.Sprintf("Block_%s", strings.TrimPrefix(url, "http://")), func(t *testing.T) {
			taskID := createTask(t)
			addFileToTask(t, taskID, url)
			resp := getTaskStatus(t, taskID)

			// Ожидаем ошибку — 403 Forbidden
			if status := resp.Task.Files[0].Status; status != http.StatusForbidden {
				t.Errorf("Expected 403 for private URL %s, got %d", url, status)
			}

			deleteTask(t, taskID)
		})
	}
}

func TestBlockDownloadFromLocalhost(t *testing.T) {
	// Поднимаем локальный файл сервер
	server := http.Server{
		Addr:    "localhost:" + fileServerPort,
		Handler: http.StripPrefix("/files/", http.FileServerFS(files.Static)),
	}
	go server.ListenAndServe()
	defer server.Shutdown(context.Background())

	// Создаем задачу на скачивание файла с localhost
	taskID := createTask(t)
	url := fmt.Sprintf("http://localhost:%s/files/jpeg.jpeg", fileServerPort)
	addFileToTask(t, taskID, url)

	// Получаем архив
	body, err := func() ([]byte, error) {
		archiveURL := fmt.Sprintf("%s/api/tasks/%d/archive", testServerURL, taskID)
		resp, err := http.Get(archiveURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}()
	if err != nil {
		t.Fatalf("Failed to download archive: %v", err)
	}

	// Получаем статус задачи и извлекаем имя файла
	taskStatus := getTaskStatus(t, taskID)

	// Проверяем, что файл отмечен как заблокированый
	var file File
	if len(taskStatus.Task.Files) == 0 {
		t.Errorf("Expected 403 for private URL %s, but status list is empty", url)
	} else {
		file = taskStatus.Task.Files[0]
		if file.Status != http.StatusForbidden {
			t.Errorf("Expected 403 for private URL %s, got %d", url, file.Status)
		}
	}
	fileName := cmp.Or(file.Name, file.OrigName, "jpeg.jpeg")

	// Проверяем, что в архиве не файла с локального хоста
	// FIXME: проверка по имени ненадежна нужно проверять хеш-сумму.
	//  Проверять, что архив пустой нельзя, т.к. loader может (и добавляет) файл отчета в архив
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("can't create zip reader: %v", err)
	}
	for _, file := range zr.File {
		if file.Name == fileName {
			t.Errorf("archive contains data file %s", fileName)
		}
	}
}

// Вспомогательные функции

func createTask(t *testing.T) int64 {
	t.Helper()
	resp, err := http.Post(testServerURL+"/api/tasks", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Create task failed with status %d", resp.StatusCode)
	}
	var createResp CreateTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		t.Fatal(err)
	}
	return createResp.TaskID
}

func deleteTask(t *testing.T, taskID int64) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/tasks/%d", testServerURL, taskID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for DELETE, got %d", resp.StatusCode)
	}
}

func addFileToTask(t *testing.T, taskID int64, url string) {
	t.Helper()
	reqBody := fmt.Sprintf(`{"url": "%s"}`, url)
	resp, err := http.Post(fmt.Sprintf("%s/api/tasks/%d/files", testServerURL, taskID), "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Add file failed with status %d", resp.StatusCode)
	}
}

func getTaskStatus(t *testing.T, taskID int64) GetTaskResponse {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/tasks/%d", testServerURL, taskID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get task status failed with status %d", resp.StatusCode)
	}
	var taskResp GetTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		t.Fatal(err)
	}
	return taskResp
}

// TestMain управляет жизненным циклом тестов.
// Создает окружение, запускает сервер перед выполнением тестов.
// Останавливает сервер и прибирается после.
func TestMain(m *testing.M) {
	code := func() int {
		// Подготовка окружения
		workDir, err := setupTestEnvironment()
		if err != nil {
			log.Fatalf("Failed to setup test environment: %v", err)
		}
		defer cleanupTestEnvironment(workDir) // Удаляем рабочий каталог после завершения

		// Запуск сервера
		serverProcess, err := startServer(binFile, workDir)
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
		defer stopServer(serverProcess) // Корректно останавливаем сервер

		// Ожидание готовности сервера
		if err := waitForServer(testServerURL + "/api/ping"); err != nil {
			log.Fatalf("Server did not become ready: %v", err)
		}

		// Запуск всех тестов
		return m.Run()
	}()

	// Выход с кодом, возвращенным тестами
	os.Exit(code)
}

// setupTestEnvironment создает рабочий каталог и необходимые подкаталоги.
func setupTestEnvironment() (string, error) {
	// Создаем временный рабочий каталог для сервера
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("Can't get work dir")
	}
	workDir := filepath.Join(wd, strconv.Itoa(rand.Int()))
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create work dir: %w", err)
	}

	log.Printf("Test environment setup complete in: %s", workDir)
	return workDir, nil
}

// cleanupTestEnvironment удаляет рабочий каталог после завершения тестов.
func cleanupTestEnvironment(workDir string) {
	if err := os.RemoveAll(workDir); err != nil {
		log.Printf("Warning: failed to cleanup test environment: %v", err)
	} else {
		log.Printf("Test environment cleaned up: %s", workDir)
	}
}

// startServer запускает бинарник zipgetd в отдельном процессе.
// Возвращает *exec.Cmd и ошибку.
func startServer(binPath, workDir string) (*exec.Cmd, error) {
	cmd := exec.Command(binPath)

	// Устанавливаем рабочий каталог для процесса
	cmd.Dir = workDir

	// Наследуем переменные окружения и добавляем тестовые
	env := os.Environ()
	for k, v := range testEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Перенаправляем stdout и stderr для логирования
	cmd.Stdout = &logWriter{prefix: "SERVER-OUT: "}
	cmd.Stderr = &logWriter{prefix: "SERVER-ERR: "}

	// Запускаем процесс
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	log.Printf("Server started with PID: %d", cmd.Process.Pid)
	return cmd, nil
}

// stopServer корректно останавливает процесс сервера.
func stopServer(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		log.Printf("No process")
		return
	}

	log.Printf("Stopping server process (PID: %d)...", cmd.Process.Pid)

	// Отправляем сигнал завершения
	err := cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		log.Printf("Failed to send SIGTERM: %v", err)
		err = cmd.Process.Signal(os.Interrupt)
	}
	if err != nil {
		log.Printf("Failed to send Interrupt: %v", err)
		err = cmd.Process.Kill()
	}

	// Если не получилось киляем
	if err != nil {
		log.Printf("Failed to kill process: %v", err)
		return
	}

	// Ожидаем завершения процесса до таймаута
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("Server process exited with error: %v", err)
		} else {
			log.Printf("Server process exited successfully")
		}
	case <-time.After(10 * time.Second):
		log.Printf("Server shutdown timeout, forcing kill...")
		cmd.Process.Kill()
	}
}

// waitForServer ожидает, пока сервер не станет доступен.
// Проверяет доступность по указанному URL.
func waitForServer(url string) error {
	const timeout = 10 * time.Second
	const interval = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tm := time.NewTimer(0)
	defer tm.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for server to be ready")
		case <-tm.C:
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					log.Printf("Server is ready at %s", url)
					return nil
				}
			}
			tm.Reset(interval)
		}
	}
}

type logWriter struct {
	prefix string
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	// Убираем символы новой строки, чтобы избежать лишних пустых строк
	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			log.Printf("%s%s", lw.prefix, line)
		}
	}
	return len(p), nil
}
