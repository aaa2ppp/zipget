package loader

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"

	"2025-07-30/internal/model"
)

const (
	bufSize         = 4096
	magicLen        = 8
)

type File = model.File

type Loader struct {
	client *http.Client
	valid  map[string]bool
}

func New(client *http.Client, validMIMETypes []string) *Loader {
	valid := make(map[string]bool, len(validMIMETypes))
	for _, contentType := range validMIMETypes {
		valid[contentType] = true
	}
	return &Loader{
		client: client,
		valid:  valid,
	}
}

func (ldr *Loader) Check(ctx context.Context, urls []string) ([]File, error) {
	var wg sync.WaitGroup
	wg.Add(len(urls))

	files := make([]File, len(urls))
	errs := make([]error, len(urls))

	for i, url := range urls {
		go func(i int, url string) {
			defer wg.Done()
			file, err := ldr.CheckFile(ctx, url, i+1)
			files[i] = file
			errs[i] = err
		}(i, url)
	}

	wg.Wait()

	return files, errors.Join(errs...)
}

func (ldr *Loader) CheckFile(ctx context.Context, uri string, uniqueNum int) (File, error) {
	log := slog.With("op", "checkFile", "url", uri)

	file := File{URL: uri}
	defer func() {
		if file.Status != http.StatusOK && file.ErrorMsg == "" {
			file.ErrorMsg = http.StatusText(file.Status)
		}
	}()

	// Валидация URL
	url, err := url.ParseRequestURI(uri)
	if err != nil {
		file.Status = http.StatusBadRequest
		file.ErrorMsg = fmt.Sprintf("invalid url: %v", err)
		log.Debug("invalid url", "error", err)
		return file, nil
	}

	// Запрос заголовков
	req, err := http.NewRequestWithContext(ctx, "HEAD", url.String(), nil)
	if err != nil {
		file.Status = http.StatusInternalServerError
		log.Error("create request failed", "error", err)
		return file, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := ldr.client.Do(req)
	if err != nil {
		file.Status = http.StatusBadGateway
		log.Debug("request failed", "error", err)
		return file, nil
	}
	resp.Body.Close()

	// Проверка статуса
	file.Status = resp.StatusCode
	if file.Status != http.StatusOK {
		log.Debug("unexpected status", "status", file.Status)
		return file, nil
	}

	file.Size = getContentLength(resp)

	// Проверка Content-Type
	file.ContentType = getContentType(resp)
	if !ldr.valid[file.ContentType] {
		file.Status = http.StatusForbidden
		file.ErrorMsg = fmt.Sprintf("file type %q is not allowed", file.ContentType)
		log.Debug("blocked by content-type", "contentType", file.ContentType)
		return file, nil
	}

	fileType, _ := getFileTypeByMIME(file.ContentType)
	file.Name = constructFileName(getFileName(resp), fileType.Extension(), uniqueNum)

	log.Debug("success")
	return file, nil
}

func (ldr *Loader) Download(ctx context.Context, urls []string, out io.Writer) ([]File, error) {
	zipWriter := zip.NewWriter(out)
	defer zipWriter.Close()

	var failed int

	files := make([]File, 0, len(urls))
	for i, url := range urls {
		file, err := ldr.downloadFile(ctx, zipWriter, url, i+1)
		files = append(files, file)

		if err != nil {
			return files, err
		}

		if file.Status != http.StatusOK {
			failed++
		}
	}

	if err := ldr.writeStatus(zipWriter, files); err != nil {
		return files, err
	}

	if failed == len(files) {
		return files, fmt.Errorf("all files were failed")
	}

	return files, nil
}

func (ldr *Loader) writeStatus(zw *zip.Writer, files []File) error {
	fw, err := zw.Create("status.json")
	if err != nil {
		return fmt.Errorf("create zip entry failed: %w", err)
	}
	cdr := json.NewEncoder(fw)
	cdr.SetIndent("", "    ")
	return cdr.Encode(files)
}

func (ldr *Loader) downloadFile(ctx context.Context, zipWriter *zip.Writer, uri string, uniqueNum int) (File, error) {
	log := slog.With("op", "downloadFile", "url", uri)

	file := File{URL: uri}
	defer func() {
		if file.Status != http.StatusOK && file.ErrorMsg == "" {
			file.ErrorMsg = http.StatusText(file.Status)
		}
	}()

	// Валидация URL
	url, err := url.ParseRequestURI(uri)
	if err != nil {
		file.Status = http.StatusBadRequest
		file.ErrorMsg = fmt.Sprintf("invalid url: %v", err)
		log.Debug("invalid url", "error", err)
		return file, nil
	}

	// Запрос файла
	req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
	if err != nil {
		file.Status = http.StatusInternalServerError
		log.Error("create request failed", "error", err)
		return file, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := ldr.client.Do(req)
	if err != nil {
		file.Status = http.StatusBadGateway
		log.Debug("request failed", "error", err)
		return file, nil
	}
	defer resp.Body.Close()

	// Проверка статуса
	file.Status = resp.StatusCode
	if file.Status != http.StatusOK {
		log.Debug("unexpected status", "status", file.Status)
		return file, nil
	}

	// Проверка Content-Type
	file.ContentType = getContentType(resp)
	if !ldr.valid[file.ContentType] {
		file.Status = http.StatusForbidden
		file.ErrorMsg = fmt.Sprintf("file type %q is not allowed", file.ContentType)
		log.Debug("blocked by content-type", "contentType", file.ContentType)
		return file, nil
	}

	buf := make([]byte, bufSize)
	var readErr error

	// Чтение первого чанка (нужен для проверки сигнатуру)
	for file.Size < magicLen && readErr == nil {
		var n int
		n, readErr = resp.Body.Read(buf[file.Size:])
		file.Size += int64(n)
	}
	if readErr != nil && readErr != io.EOF {
		file.Status = http.StatusBadGateway
		log.Debug("first chank read failed", "error", readErr)
		return file, nil
	}

	// Проверка сигнатуры
	magic := buf[:min(magicLen, file.Size)]
	fileType, err := getFileTypeBySignature(magic)
	if err != nil {
		file.Status = http.StatusForbidden
		file.ErrorMsg = err.Error()
		log.Debug("can't check real file type", "error", err)
		return file, nil
	}

	file.RealType = fileType.MIMEType
	if !ldr.valid[file.RealType] {
		file.Status = http.StatusForbidden
		file.ErrorMsg = fmt.Sprintf("file type %q is not allowed", file.RealType)
		log.Debug("blocked by real file type", "realType", file.RealType)
		return file, nil
	}

	// Создание файла в архиве
	file.Name = constructFileName(getFileName(resp), fileType.Extension(), uniqueNum)
	fileWriter, err := zipWriter.Create(file.Name)
	if err != nil {
		file.Status = http.StatusInternalServerError
		log.Error("create zip entry failed", "error", err)
		return file, fmt.Errorf("create zip entry failed: %w", err)
	}

	// Запись первого чанка
	if file.Size > 0 {
		if _, err := fileWriter.Write(buf[:file.Size]); err != nil {
			file.Status = http.StatusInternalServerError
			log.Error("write failed", "error", err)
			return file, fmt.Errorf("write failed: %w", err)
		}
	}

	// Копирование оставшихся данных
	for readErr == nil {
		var n int
		n, readErr = resp.Body.Read(buf)
		if n == 0 {
			continue
		}
		file.Size += int64(n)

		if _, err := fileWriter.Write(buf[:n]); err != nil {
			file.Status = http.StatusInternalServerError
			log.Error("write failed", "error", err)
			return file, fmt.Errorf("write failed: %w", err)
		}
	}

	if readErr != io.EOF {
		file.Status = http.StatusBadGateway
		log.Debug("read failed", "error", readErr)
		return file, nil
	}

	log.Debug("success")
	return file, nil
}
