package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"zipget/internal/logger"
	"zipget/internal/model"
)

const (
	numberOfFilesToShowArchiveURL = 3
)

type Manager interface {
	CreateTask(ctx context.Context) (int64, error)
	DeleteTask(ctx context.Context, taskID int64) error
	AddFileToTask(ctx context.Context, taskID int64, url string) error
	GetTaskStatus(ctx context.Context, taskID int64) (model.Task, error)
	ProcessTask(ctx context.Context, taskID int64, out io.Writer) error
}

func New(manager Manager, apiBasePath, filesBasePath string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST " /****/ +apiBasePath+"/tasks", CreateTask(manager))
	mux.HandleFunc("DELETE " /**/ +apiBasePath+"/tasks/{id}", DeleteTask(manager))
	mux.HandleFunc("GET " /*****/ +apiBasePath+"/tasks/{id}", GetTaskStatus(manager, filesBasePath))
	mux.HandleFunc("POST " /****/ +apiBasePath+"/tasks/{id}/files", AddFileToTask(manager))
	mux.HandleFunc("GET " /*****/ +apiBasePath+"/tasks/{id}/archive", ProcessTask(manager))
	mux.Handle("GET "+filesBasePath+"/", GetArchive(filesBasePath))
	return mux
}

type createTaskResponse struct {
	TaskID int64 `json:"task_id"`
}

func CreateTask(m Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := newHelper(w, r, "CreateTask")

		taskID, err := m.CreateTask(h.Ctx())
		if err != nil {
			h.WriteError(err)
			return
		}

		resp := createTaskResponse{TaskID: taskID}
		h.WriteResponse(resp, http.StatusCreated)
	}
}

func DeleteTask(m Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := newHelper(w, r, "DeleteTask")

		taskID, err := h.GetID()
		if err != nil {
			h.WriteError(err)
			return
		}

		if err := m.DeleteTask(h.Ctx(), taskID); err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(struct{}{}, http.StatusOK)
	}
}

type addFileToTaskRequest struct {
	URL string `json:"url,omitempty"`
}

func AddFileToTask(m Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := newHelper(w, r, "AddFileToTask")

		taskID, err := h.GetID()
		if err != nil {
			h.WriteError(err)
			return
		}

		var req addFileToTaskRequest
		if err := h.ReadRequest(&req); err != nil {
			h.WriteError(err)
			return
		}

		if req.URL == "" {
			h.WriteError(&httpError{
				StatusCode: http.StatusBadRequest,
				StatusMsg:  "url is required",
			})
			return
		}

		if err := m.AddFileToTask(h.Ctx(), taskID, req.URL); err != nil {
			h.WriteError(err)
			return
		}

		h.WriteResponse(struct{}{}, http.StatusOK)
	}
}

type getTaskStatusResponse struct {
	Task    model.Task `json:"task"`
	Archive string     `json:"archive,omitempty"`
}

func GetTaskStatus(m Manager, filesBasePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := newHelper(w, r, "GetTaskStatus")

		taskID, err := h.GetID()
		if err != nil {
			h.WriteError(err)
			return
		}

		task, err := m.GetTaskStatus(h.Ctx(), taskID)
		if err != nil {
			h.WriteError(err)
			return
		}

		resp := getTaskStatusResponse{Task: task}

		// XXX чтобы удовлетворить требовние ТЗ:
		// "Как только число добавляемых файлов в задачу будет равно трем, метод получения
		// статуса должен, вместе со статусом, вернуть ссылку на архив."
		if len(task.Files) >= numberOfFilesToShowArchiveURL {
			resp.Archive = fmt.Sprintf("%s/task_%d.zip", filesBasePath, taskID)
		}

		h.WriteResponse(resp, http.StatusOK)
	}
}

func ProcessTask(m Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := newHelper(w, r, "DownloadTaskFiles")

		taskID, err := h.GetID()
		if err != nil {
			h.WriteError(err)
			return
		}

		if _, err := m.GetTaskStatus(h.Ctx(), taskID); err != nil {
			h.WriteError(err)
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="task_%d.zip"`, taskID))
		w.WriteHeader(http.StatusOK)

		bw := bufio.NewWriterSize(w, 64*1024)
		defer bw.Flush()

		if err := m.ProcessTask(h.Ctx(), taskID, bw); err != nil {
			h.log.Error("process task failed", "error", err)
			return
		}
	}
}

func GetArchive(filesBasePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		if dir := path.Dir(r.URL.Path); dir != filesBasePath {
			log.Debug("invalid dir", "dir", dir)
			http.NotFound(w, r)
			return
		}
		taskStr := path.Base(r.URL.Path)

		if !strings.HasSuffix(taskStr, ".zip") {
			log.Debug("must be suffix .zip", "taskStr", taskStr)
			http.NotFound(w, r)
			return
		}
		taskStr = strings.TrimSuffix(taskStr, ".zip")

		if !strings.HasPrefix(taskStr, "task_") {
			log.Debug("must be prefix task_", "taskStr", taskStr)
			http.NotFound(w, r)
			return
		}
		taskStr = strings.TrimPrefix(taskStr, "task_")

		taskID, err := strconv.ParseInt(taskStr, 10, 64)
		if err != nil {
			log.Debug("can't parse taskID", "taskID", taskStr)
			http.NotFound(w, r)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/api/tasks/%d/archive", taskID), http.StatusTemporaryRedirect)
	}
}
