package loader

import (
	"bytes"
	"errors"
)

type FileType struct {
	MIMEType   string
	Magic      []byte // сигнатура файла
	Extensions []string
}

func (f FileType) Extension() string {
	if len(f.Extensions) == 0 {
		return ""
	}
	return f.Extensions[0]
}

var fileTypes = []FileType{
	{
		MIMEType:   "image/jpeg",
		Magic:      []byte{0xFF, 0xD8, 0xFF}, // ÿØÿ
		Extensions: []string{".jpg", ".jpeg"},
	},
	{
		MIMEType:   "image/png",
		Magic:      []byte{0x89, 0x50, 0x4E, 0x47}, // ‰PNG
		Extensions: []string{".png"},
	},
	{
		MIMEType:   "image/gif",
		Magic:      []byte{0x47, 0x49, 0x46, 0x38}, // GIF8
		Extensions: []string{".gif"},
	},
	{
		MIMEType:   "application/pdf",
		Magic:      []byte{0x25, 0x50, 0x44, 0x46}, // %PDF
		Extensions: []string{".pdf"},
	},
	{
		MIMEType:   "application/zip",
		Magic:      []byte{0x50, 0x4B, 0x03, 0x04}, // PK
		Extensions: []string{".zip"},
	},
	// ...
}

var ErrUnknownFileType = errors.New("unknown file type")

func getFileTypeBySignature(magic []byte) (FileType, error) {
	for _, ft := range fileTypes {
		if bytes.HasPrefix(magic, ft.Magic) {
			return ft, nil
		}
	}
	return FileType{}, ErrUnknownFileType
}

func getFileTypeByMIME(mimeType string) (FileType, error) {
	for _, ft := range fileTypes {
		if ft.MIMEType == mimeType {
			return ft, nil
		}
	}
	return FileType{}, ErrUnknownFileType
}
