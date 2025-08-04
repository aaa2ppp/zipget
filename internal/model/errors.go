package model

import "errors"

var (
	ErrTaskNotFound     = errors.New("task not found")
	ErrMaxFilesExceeded = errors.New("maximum files exceeded")
	ErrServerBusy       = errors.New("server busy")
	ErrServerCancelled  = errors.New("server has been cancelled")
)
