package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogLevel represents the level of logging
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn  
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging functionality
type Logger struct {
	level  LogLevel
	output io.Writer
	prefix string
}

// defaultLogger is the package-level logger instance
var defaultLogger *Logger

// init initializes the default logger
func init() {
	defaultLogger = New(LevelInfo, os.Stderr, "gci")
}

// New creates a new logger instance
func New(level LogLevel, output io.Writer, prefix string) *Logger {
	return &Logger{
		level:  level,
		output: output,
		prefix: prefix,
	}
}

// SetLevel sets the logging level for the default logger
func SetLevel(level LogLevel) {
	defaultLogger.level = level
}

// SetVerbose enables verbose logging (DEBUG level) to stderr
func SetVerbose(verbose bool) {
	if verbose {
		defaultLogger.level = LevelDebug
		// In verbose mode, also log to file for debugging
		logFile := getDebugLogFile()
		if logFile != nil {
			defaultLogger.output = io.MultiWriter(os.Stderr, logFile)
		}
	} else {
		defaultLogger.level = LevelInfo
		defaultLogger.output = os.Stderr
	}
}

// getDebugLogFile returns a file handle for debug logging
func getDebugLogFile() *os.File {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	
	logPath := filepath.Join(home, ".config", "gci_debug.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return nil
	}
	
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil
	}
	
	return file
}

// log is the core logging function
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}
	
	timestamp := time.Now().Format("2006-01-02T15:04:05")
	message := fmt.Sprintf(format, args...)
	
	// Structured log format: timestamp level [prefix] message
	logLine := fmt.Sprintf("%s %s [%s] %s\n", timestamp, level.String(), l.prefix, message)
	
	// Filter out secrets - never log tokens, passwords, or auth headers
	if containsSensitive(message) {
		logLine = fmt.Sprintf("%s %s [%s] %s\n", timestamp, level.String(), l.prefix, "[REDACTED: contains sensitive data]")
	}
	
	l.output.Write([]byte(logLine))
}

// containsSensitive checks if a message contains sensitive information
func containsSensitive(message string) bool {
	lower := strings.ToLower(message)
	sensitiveWords := []string{
		"token", "password", "apikey", "api_key", "auth", "credential", 
		"secret", "key=", "authorization:", "basic ", "bearer ",
	}
	
	for _, word := range sensitiveWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// Package-level logging functions

// Debug logs debug information (only shown with --verbose)
func Debug(format string, args ...interface{}) {
	defaultLogger.log(LevelDebug, format, args...)
}

// Info logs informational messages  
func Info(format string, args ...interface{}) {
	defaultLogger.log(LevelInfo, format, args...)
}

// Warn logs warning messages
func Warn(format string, args ...interface{}) {
	defaultLogger.log(LevelWarn, format, args...)
}

// Error logs error messages
func Error(format string, args ...interface{}) {
	defaultLogger.log(LevelError, format, args...)
}

// HTTP logs HTTP request/response information (debug level)
func HTTP(method, url string) {
	Debug("HTTP %s %s", method, url)
}

// HTTPResponse logs HTTP response information (debug level)
func HTTPResponse(status int, duration time.Duration) {
	Debug("HTTP response: %d (%v)", status, duration)
}

// Config logs configuration-related information (debug level)
func Config(format string, args ...interface{}) {
	Debug("CONFIG: "+format, args...)
}

// TUI logs TUI-related information (debug level)
func TUI(format string, args ...interface{}) {
	Debug("TUI: "+format, args...)
}

// JIRA logs JIRA API-related information (debug level)
func JIRA(format string, args ...interface{}) {
	Debug("JIRA: "+format, args...)
}