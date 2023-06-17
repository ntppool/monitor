package logger

import (
	"fmt"

	"golang.org/x/exp/slog"
)

type stdLoggerish struct {
	key string
	log *slog.Logger
	f   func(string, ...any)
}

func NewStdLog(key string, debug bool, log *slog.Logger) *stdLoggerish {
	if log == nil {
		log = Setup()
	}
	sl := &stdLoggerish{
		key: key,
		log: log,
	}
	sl.f = log.Info
	if debug {
		sl.f = log.Debug
	}

	return sl
}

func (l stdLoggerish) Println(msg ...interface{}) {
	l.f(l.key, "msg", msg)
}

func (l stdLoggerish) Printf(msg string, args ...interface{}) {
	l.f(l.key, "msg", fmt.Sprintf(msg, args...))
}
