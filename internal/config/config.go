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
	var ge getenv
	cfg := Config{
		Logger: Logger{
			Level:     ge.LogLevel("LOG_LEVEL", false, slog.LevelInfo),
			Plaintext: ge.Bool("LOG_PLAINTEXT", false, false),
		},
		Server: Server{
			Addr: ge.String("SERVER_ADDR", false, ":8080"),
		},
		Manager: Manager{
			MaxTotal:  ge.Int("MANAGER_MAX_TOTAL", false, 100500),
			MaxActive: ge.Int("MANAGER_MAX_ACTIVE", false, 3),
			MaxFiles:  ge.Int("MANAGER_MAX_FILES", false, 3),
			TaskTTL:   ge.Duration("MANAGER_TASK_TTL", false, 1*time.Hour),
		},
		Loader: Loader{
			AllowMIMETypes: ge.Strings("LOADER_ALLOW_MIME", false, []string{
				"application/pdf",
				"image/jpeg",
			}),
		},
	}
	return cfg, ge.Err()
}
