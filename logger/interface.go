package logger

type Interface interface {
	Info(v string)
	Infof(format string, args ...any)
	Warn(v string)
	Warnf(format string, args ...any)
	Error(v string)
	Errorf(format string, args ...any)
	Fatal(v string)
	Fatalf(format string, args ...any)
}

