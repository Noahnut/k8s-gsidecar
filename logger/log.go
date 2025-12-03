package logger

import (
	"log/slog"
	"os"
)

var logger *slog.Logger

func init() {
	log_level := os.Getenv("LOG_LEVEL")
	if log_level == "" {
		log_level = "INFO"
	}

	level := SetLogLevel(log_level)

	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

func SetLogLevel(level string) slog.Level {
	var log_level slog.Level

	switch level {
	case "DEBUG":
		log_level = slog.LevelDebug
	case "INFO":
		log_level = slog.LevelInfo
	case "WARN":
		log_level = slog.LevelWarn
	case "ERROR":
		log_level = slog.LevelError
	default:
		log_level = slog.LevelInfo
	}

	return log_level
}

func GetLogger() *slog.Logger {
	return logger
}
