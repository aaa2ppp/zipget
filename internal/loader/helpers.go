package loader

import (
	"mime"
	"net/http"
	"strconv"
	"strings"
)

func getContentLength(resp *http.Response) int64 {
	sizeStr := resp.Header.Get("Content-Length")
	if sizeStr == "" {
		return 0
	}
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return 0
	}
	return size
}

func getContentType(resp *http.Response) string {
	contentType := resp.Header.Get("Content-Type")
	if end := strings.IndexByte(contentType, ';'); end != -1 {
		contentType = strings.TrimSpace(contentType[:end])
	}
	return contentType
}

func getFileName(resp *http.Response) string {
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err != nil {
		return ""
	}
	if fileName, ok := params["filename"]; ok {
		return fileName
	}
	return ""
}
