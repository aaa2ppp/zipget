package config

import (
	"log/slog"
	"time"
)

type Logger struct {
	Level     slog.Level
	Plaintext bool
}

type Server struct {
	Addr string
}

type Manager struct {
	MaxTotal  int           // максимальное количество задач
	MaxActive int           // максимальное количество активных загрузок
	MaxFiles  int           // максимальное количество URLs на задачу
	TaskTTL   time.Duration // время жизни задачи
}

type Loader struct {
	AllowMIMETypes []string
}

type Config struct {
	Logger  Logger
	Server  Server
	Manager Manager
	Loader  Loader
}

func Load() (Config, error) {
	const required = true
	var ge getenv
	cfg := Config{
		Logger: Logger{
			Level:     ge.LogLevel("LOG_LEVEL", !required, slog.LevelInfo),
			Plaintext: ge.Bool("LOG_PLAINTEXT", !required, false),
		},
		Server: Server{
			Addr: ge.String("SERVER_ADDR", !required, ":8080"),
		},
		Manager: Manager{
			MaxTotal:  ge.Int("MANAGER_MAX_TOTAL", !required, 1000),
			MaxActive: ge.Int("MANAGER_MAX_ACTIVE", !required, 3),
			MaxFiles:  ge.Int("MANAGER_MAX_FILES", !required, 3),
			TaskTTL:   ge.Duration("MANAGER_TASK_TTL", !required, 10*time.Minute),
		},
		Loader: Loader{
			AllowMIMETypes: ge.Strings("LOADER_ALLOW_MIME", required, nil),
		},
	}
	return cfg, ge.Err()
}
