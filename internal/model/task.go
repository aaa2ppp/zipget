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

func (t Task) Clone() Task {
	return Task{
		ID:        t.ID,
		Files:     slices.Clone(t.Files),
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
		ExpiresAt: t.ExpiresAt,
	}
}
