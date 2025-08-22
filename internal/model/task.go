package model

import (
	"slices"
	"time"
)

type Task struct {
	ID        int64     `json:"id,omitempty"`
	Files     []File    `json:"files,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

// Clone создает полную копию задачи, включая глубокое копирование слайса Files.
// Нужен для безопасного возврата состояния задачи без риска изменения внутреннего состояния хранилища.
func (t Task) Clone() Task {
	return Task{
		ID:        t.ID,
		Files:     slices.Clone(t.Files),
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
		ExpiresAt: t.ExpiresAt,
	}
}
