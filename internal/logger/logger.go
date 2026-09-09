// Package logger 提供带级别的轻量日志（标准库实现，零依赖）。
package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Level 日志级别
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	mu       sync.Mutex
	curLevel = LevelInfo
	std      = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
)

// SetLevel 设置全局日志级别，接受 debug/info/warn/error（大小写不敏感）。
func SetLevel(s string) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		curLevel = LevelDebug
	case "warn", "warning":
		curLevel = LevelWarn
	case "error":
		curLevel = LevelError
	default:
		curLevel = LevelInfo
	}
}

func output(lv Level, tag, format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	if lv < curLevel {
		return
	}
	msg := fmt.Sprintf(format, args...)
	std.Printf("[%s] %s", tag, msg)
}

// Debugf 调试日志
func Debugf(format string, args ...interface{}) { output(LevelDebug, "DBG", format, args...) }

// Infof 信息日志
func Infof(format string, args ...interface{}) { output(LevelInfo, "INF", format, args...) }

// Warnf 警告日志
func Warnf(format string, args ...interface{}) { output(LevelWarn, "WRN", format, args...) }

// Errorf 错误日志
func Errorf(format string, args ...interface{}) { output(LevelError, "ERR", format, args...) }
