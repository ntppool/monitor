package logger

import (
	"fmt"

	"golang.org/x/exp/slog"
)

type stdLoggerish struct {
	key string
	log *slog.Logger
}

func NewStdLog(key string, log *slog.Logger) *stdLoggerish {
	if log == nil {
		log = Setup()
	}
	return &stdLoggerish{
		key: key,
		log: log,
	}
}

func (l stdLoggerish) Println(msg ...interface{}) {
	l.log.Info(l.key, "msg", msg)
}

func (l stdLoggerish) Printf(msg string, args ...interface{}) {
	l.log.Info(l.key, "msg", fmt.Sprintf(msg, args...))
}
