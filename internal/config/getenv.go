package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrEnvRequired = errors.New("env is required")

type getenv struct {
	errs []error
}

func (ge *getenv) Err() error {
	return errors.Join(ge.errs...)
}

func (ge *getenv) String(key string, required bool, defaultValue string) string {
	if s, ok := os.LookupEnv(key); ok {
		return s
	}

	if required {
		ge.errs = append(ge.errs, fmt.Errorf("%s %w", key, ErrEnvRequired))
		return ""
	}

	return defaultValue
}

func (ge *getenv) Strings(key string, required bool, defaultValue []string) []string {
	if s, ok := os.LookupEnv(key); ok {
		return strings.Fields(s)
	}

	if required {
		ge.errs = append(ge.errs, fmt.Errorf("%s %w", key, ErrEnvRequired))
		return nil
	}

	return defaultValue
}

func (ge *getenv) Int(key string, required bool, defaultValue int) int {
	if s, ok := os.LookupEnv(key); ok {
		v, err := strconv.Atoi(s)
		if err != nil {
			ge.errs = append(ge.errs, err)
			return 0
		}
		return v
	}

	if required {
		ge.errs = append(ge.errs, fmt.Errorf("%s %w", key, ErrEnvRequired))
		return 0
	}

	return defaultValue
}

func (ge *getenv) LogLevel(key string, required bool, defaultValue slog.Level) slog.Level {
	if s, ok := os.LookupEnv(key); ok {
		var v slog.Level
		if err := v.UnmarshalText([]byte(s)); err != nil {
			ge.errs = append(ge.errs, err)
			return 0
		}
		return v
	}

	if required {
		ge.errs = append(ge.errs, fmt.Errorf("%s %w", key, ErrEnvRequired))
		return 0
	}

	return defaultValue
}

func (ge *getenv) Bool(key string, required bool, defaultValue bool) bool {
	if s, ok := os.LookupEnv(key); ok {

		switch strings.ToLower(s) {
		case "true", "yes", "on", "1":
			return true
		case "false", "no", "off", "0":
			return false
		default:
			msg := fmt.Sprintf("%s=%s env is ignored. Want value: true/false, yes/no, on/off or 1/0", key, s)
			if required {
				ge.errs = append(ge.errs, errors.New(msg))
			} else {
				slog.Error(msg)
			}
			return false
		}

	}

	if required {
		ge.errs = append(ge.errs, fmt.Errorf("%s %w", key, ErrEnvRequired))
		return false
	}

	return defaultValue
}

func (ge *getenv) Duration(key string, required bool, defaultValue time.Duration) time.Duration {
	if s, ok := os.LookupEnv(key); ok {
		v, err := time.ParseDuration(s)
		if err != nil {
			ge.errs = append(ge.errs, err)
			return 0
		}
		return v
	}

	if required {
		ge.errs = append(ge.errs, fmt.Errorf("%s %w", key, ErrEnvRequired))
		return 0
	}

	return defaultValue
}
