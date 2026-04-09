package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger 日志记录器
type Logger struct {
	file     *os.File
	filePath string
	mu       sync.Mutex
}

var (
	instance *Logger
	once     sync.Once
)

// GetLogger 获取日志记录器单例
func GetLogger() *Logger {
	once.Do(func() {
		// 创建日志目录
		homeDir, _ := os.UserHomeDir()
		logDir := filepath.Join(homeDir, ".pan-player", "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			logDir = "/tmp"
		}

		// 创建日志文件
		logPath := filepath.Join(logDir, fmt.Sprintf("pan-player_%s.log", time.Now().Format("2006-01-02")))
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// 如果创建失败，使用临时目录
			logPath = filepath.Join("/tmp", fmt.Sprintf("pan-player_%s.log", time.Now().Format("2006-01-02")))
			file, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				file = nil // 如果还是失败，至少不会崩溃
			}
		}

		instance = &Logger{
			file:     file,
			filePath: logPath,
		}

		// 输出日志文件位置到stderr（不会干扰UI）
		if file != nil {
			fmt.Fprintf(os.Stderr, "日志文件: %s\n", logPath)
		}
	})

	return instance
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log("DEBUG", format, args...)
}

// Info 信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	l.log("INFO", format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log("WARN", format, args...)
}

// Error 错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	l.log("ERROR", format, args...)
}

// log 内部日志记录方法
func (l *Logger) log(level string, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02T15:04:05.000")
	logLine := fmt.Sprintf("[%s] %s %s\n", level, timestamp, fmt.Sprintf(format, args...))

	if l.file != nil {
		if _, err := l.file.WriteString(logLine); err != nil {
			// 如果写入失败，输出到stderr
			fmt.Fprintf(os.Stderr, logLine)
		}
	} else {
		// 如果文件不可用，输出到stderr
		fmt.Fprintf(os.Stderr, logLine)
	}
}

// GetLogPath 获取日志文件路径
func (l *Logger) GetLogPath() string {
	return l.filePath
}
