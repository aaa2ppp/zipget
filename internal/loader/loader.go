package loader

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"zipget/internal/logger"
	"zipget/internal/model"
	"zipget/internal/protect"
)

const (
	bufSize  = 4096
	magicLen = 8
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

// Check параллельно проверяет доступность и валидность списка URL с помощью HTTP HEAD-запросов.
//
// Для каждого URL создаётся отдельный поток выполнения. Результаты собираются в срез []File
// в том же порядке, что и входной срез urls. Даже если проверка некоторых URL завершается с ошибкой,
// функция всё равно возвращает полный срез с заполненными полями Status, ErrorMsg и др.
//
// Возвращает:
//   - []File: один файл на каждый URL, в том же порядке.
//   - error: объединённая ошибка (errors.Join) всех фатальных ошибок (например, контекст отменён,
//     ошибка при создании запроса). Некритические ошибки (например, 404, 502) не возвращаются
//     как error, а отражаются в полях Status и ErrorMsg соответствующего File.
//   - Если передан пустой список urls функция ничего не делает и возвращет nil, nil.
//
// Вызывающий код должен анализировать Status каждого File, чтобы определить результат проверки.
// Например, статусы 200, 403, 404, 502 и др. означают завершение проверки с соответствующим кодом.
//
// Порядок важен: результаты сопоставляются с исходными URL по индексу.
// Проверка прерывается, если контекст отменён.
func (ldr *Loader) Check(ctx context.Context, urls []string) ([]File, error) {
	if len(urls) == 0 {
		return nil, nil
	}

	if len(urls) == 1 {
		file, err := ldr.CheckFile(ctx, urls[0])
		return []File{file}, err
	}

	var wg sync.WaitGroup
	wg.Add(len(urls))

	files := make([]File, len(urls))
	errs := make([]error, len(urls))

	for i, url := range urls {
		go func(i int, url string) {
			defer wg.Done()
			file, err := ldr.CheckFile(ctx, url)
			files[i] = file
			errs[i] = err
		}(i, url)
	}

	wg.Wait()

	return files, errors.Join(errs...)
}

// CheckFile проверяет один URL с помощью HEAD-запроса и возвращает информацию о файле.
//
// Основные этапы:
//  1. Валидация URL (должен быть корректным URI).
//  2. Отправка HEAD-запроса с использованием контекста.
//  3. Проверка HTTP-статуса (ожидается 200 OK).
//  4. Проверка Content-Type (должен быть разрешён).
//  5. Получение оригинального имени из заголовка Content-Disposition.
//
// Параметры:
//   - ctx: контекст для отмены и таймаута.
//   - uri: строка URL для проверки.
//
// Возвращает:
//   - File: структура с заполненными полями URL, Status, ContentType, Size, Name, ErrorMsg.
//     Если проверка прошла успешно (Status == 200), то файл признан доступным и разрешённого типа.
//   - error: только в случае фатальной ошибки (например, ошибка создания запроса).
//     Ошибки сети, HTTP-ошибки (4xx, 5xx), запрещённый тип — не возвращаются как error,
//     а отражаются в полях Status и ErrorMsg.
//
// Особенности:
//   - Тело ответа не читается (HEAD-запрос).
//   - Если Status не 200, ErrorMsg автоматически заполняется текстом статуса (например, "Not Found").
//
// Пример результата:
//
//	File{URL: "http://...", Status: 200, ContentType: "image/jpeg", Size: 10240, Name: "file-1.jpg"}
func (ldr *Loader) CheckFile(ctx context.Context, uri string) (file File, _ error) {
	log := logger.FromContext(ctx).With("op", "checkFile", "fileURL", uri)

	file = File{URL: uri}
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
		if errors.Is(err, protect.ErrSSRF) {
			file.Status = http.StatusForbidden
			log.Warn("SSRF attack blocked", "error", err)
			return file, nil
		}
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

	file.OrigName = getFileName(resp)

	log.Debug("success")
	return file, nil
}

// Download скачивает файлы по указанным URL и упаковывает их в ZIP-архив, записывая в out.
//
// Для каждого URL:
//  1. Выполняется GET-запрос.
//  2. Проверяется HTTP-статус (ожидается 200 OK).
//  3. Проверяется Content-Type (должен быть разрешён).
//  4. Читается первые 8 байт (магическая сигнатура) для определения реального типа файла.
//  5. Если реальный тип не разрешён — загрузка прерывается с ошибкой.
//  6. Файл записывается в ZIP-архив с уникальным именем.
//
// Дополнительно:
//   - В архив добавляется файл `status.json` с информацией о всех загруженных файлах
//     (включая те, что не были загружены).
//   - Все файлы именуются по шаблону: <basename>-<uniqueNum>.<ext>.
//
// Параметры:
//   - ctx: контекст с таймаутом и возможностью отмены.
//   - urls: список URL для загрузки.
//   - out: io.Writer, куда будет записан ZIP-архив (например, http.ResponseWriter).
//
// Возвращает:
//   - []File: информация о каждом файле в том же порядке, что и urls.
//     Содержит URL, статус, размер, имя в архиве, типы, ошибки.
//   - error: возвращается только при критической ошибке:
//   - Ошибка записи в ZIP (например, disk full).
//   - Ошибка при создании записи в архиве.
//     Частичные ошибки (один из многих URL недоступен) - не считаются фатальными;
//   - Всегда создает архив. Если передан пустой список urls будет создан пустой архив с пустым файлом статуса.
//
// Особенности:
//   - Загрузка происходит последовательно (не параллельно), чтобы избежать перегрузки памяти.
//   - При ошибках чтения тела файла (например, обрыв соединения) — статус устанавливается в 502.
//   - После успешной загрузки одного файла, процесс продолжается со следующим.
//   - Даже если все файлы провалились, `status.json` всё равно записывается.
//
// Примечание: вызывающий код должен обрабатывать как возвращённый срез File,
// так и наличие ошибки — они не взаимоисключающие.
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

func (ldr *Loader) downloadFile(ctx context.Context, zipWriter *zip.Writer, uri string, uniqueNum int) (file File, _ error) {
	log := logger.FromContext(ctx).With("op", "downloadFile", "fileURL", uri)

	file = File{URL: uri}
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
		if errors.Is(err, protect.ErrSSRF) {
			file.Status = http.StatusForbidden
			log.Warn("SSRF attack blocked", "error", err)
			return file, nil
		}
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

	file.OrigName = getFileName(resp)

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
	file.Name = constructFileName(file.OrigName, fileType.Extension(), uniqueNum)
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
