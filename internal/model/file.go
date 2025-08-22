package model

// File представляет файл в задаче.
//
// Гарантируется, что ID уникален в пределах одной задачи.
// ID присваивается при добавлении файла и не меняется.
// Используется для безопасного обновления состояния файла без зависимости от порядка в слайсе.
type File struct {
	ID          int64  `json:"-"` // Уникален внутри задачи
	URL         string `json:"url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	RealType    string `json:"real_type,omitempty"`
	OrigName    string `json:"orig_name,omitempty"`
	Name        string `json:"name,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Status      int    `json:"status,omitempty"`
	ErrorMsg    string `json:"error_msg,omitempty"`
}
