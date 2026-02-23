package logger

import (
	"fmt"
	"time"
)

type Logger struct {
}

var _ Interface = (*Logger)(nil)

func New() *Logger {
	return &Logger{}
}

func (l *Logger) Info(v string) {
	now := time.Now()
	fmt.Printf(
		"%s [INFO] %s\n",
		now.Format("2006-01-02 15:04:05"),
		v,
	)
}

func (l *Logger) Infof(format string, args ...any) {
	l.Info(fmt.Sprintf(format, args...))
}

func (l *Logger) Warn(v string) {
	now := time.Now()
	fmt.Printf(
		"%s [WARN] %s\n",
		now.Format("2006-01-02 15:04:05"),
		v,
	)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.Warn(fmt.Sprintf(format, args...))
}

func (l *Logger) Error(v string) {
	now := time.Now()
	fmt.Printf(
		"%s [ERROR] %s\n",
		now.Format("2006-01-02 15:04:05"),
		v,
	)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.Error(fmt.Sprintf(format, args...))
}

func (l *Logger) Fatal(v string) {
	now := time.Now()
	fmt.Printf(
		"%s [FATAL] %s\n",
		now.Format("2006-01-02 15:04:05"),
		v,
	)
	panic(v)
}

func (l *Logger) Fatalf(format string, args ...any) {
	l.Fatal(fmt.Sprintf(format, args...))
}
