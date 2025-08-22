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

type parseFunc[T any] func(s string) (T, error)

func getValue[T any](key string, required bool, defaultValue T, parse parseFunc[T]) (T, error) {
	s, ok := os.LookupEnv(key)
	if !ok || s == "" {
		if required {
			var zero T
			return zero, fmt.Errorf("%s %w", key, ErrEnvRequired)
		}
		return defaultValue, nil
	}
	return parse(s)
}

func (ge *getenv) String(key string, required bool, defaultValue string) string {
	v, err := getValue(key, required, defaultValue, func(s string) (string, error) {
		return s, nil
	})
	if err != nil {
		ge.errs = append(ge.errs, err)
	}
	return v
}

func (ge *getenv) Strings(key string, required bool, defaultValue []string) []string {
	v, err := getValue(key, required, defaultValue, func(s string) ([]string, error) {
		return strings.Fields(s), nil
	})
	if err != nil {
		ge.errs = append(ge.errs, err)
	}
	return v
}

func (ge *getenv) Int(key string, required bool, defaultValue int) int {
	v, err := getValue(key, required, defaultValue, func(s string) (int, error) {
		return strconv.Atoi(s)
	})
	if err != nil {
		ge.errs = append(ge.errs, err)
	}
	return v
}

func (ge *getenv) LogLevel(key string, required bool, defaultValue slog.Level) slog.Level {
	v, err := getValue(key, required, defaultValue, func(s string) (slog.Level, error) {
		var v slog.Level
		err := v.UnmarshalText([]byte(s))
		return v, err
	})
	if err != nil {
		ge.errs = append(ge.errs, err)
	}
	return v
}

func (ge *getenv) Bool(key string, required bool, defaultValue bool) bool {
	v, err := getValue(key, required, defaultValue, func(s string) (bool, error) {
		switch strings.ToLower(s) {
		case "true", "yes", "on", "1":
			return true, nil
		case "false", "no", "off", "0":
			return false, nil
		default:
			return false, fmt.Errorf("invalid boolean value %q for %q, want: true/false, yes/no, on/off, 1/0", s, key)
		}
	})
	if err != nil {
		ge.errs = append(ge.errs, err)
	}
	return v
}

func (ge *getenv) Duration(key string, required bool, defaultValue time.Duration) time.Duration {
	v, err := getValue(key, required, defaultValue, func(s string) (time.Duration, error) {
		return time.ParseDuration(s)
	})
	if err != nil {
		ge.errs = append(ge.errs, err)
	}
	return v
}
