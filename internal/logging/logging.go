package logging

import (
	"fmt"
	"os"
	"strings"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel = LevelInfo

func SetLevel(level Level) {
	currentLevel = level
}

func SetLevelFromString(s string) {
	switch strings.ToLower(s) {
	case "debug":
		currentLevel = LevelDebug
	case "info":
		currentLevel = LevelInfo
	case "warn", "warning":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	default:
		currentLevel = LevelInfo
	}
}

func Debug(format string, args ...interface{}) {
	if currentLevel <= LevelDebug {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

func Info(format string, args ...interface{}) {
	if currentLevel <= LevelInfo {
		fmt.Printf("[INFO] "+format+"\n", args...)
	}
}

func Warn(format string, args ...interface{}) {
	if currentLevel <= LevelWarn {
		fmt.Printf("[WARN] "+format+"\n", args...)
	}
}

func Error(format string, args ...interface{}) {
	if currentLevel <= LevelError {
		fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
	}
}

func GetLevelFromEnv() Level {
	level := os.Getenv("LOG_LEVEL")
	SetLevelFromString(level)
	return currentLevel
}
