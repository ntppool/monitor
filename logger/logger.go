package logger

import (
	"context"
	"log"
	"os"
	"sync"

	"golang.org/x/exp/slog"
)

var rootLogger *slog.Logger
var setup sync.Once

func Setup() *slog.Logger {

	setup.Do(func() {

		var programLevel = new(slog.LevelVar) // Info by default

		// temp -- should be an option, and maybe have a runtime signal to adjust?
		// programLevel.Set(slog.LevelDebug)

		logOptions := &slog.HandlerOptions{Level: programLevel}

		if len(os.Getenv("INVOCATION_ID")) > 0 {
			// don't add timestamps when running under systemd
			log.Default().SetFlags(0)

			logReplace := func(groups []string, a slog.Attr) slog.Attr {
				// Remove time
				if a.Key == slog.TimeKey && len(groups) == 0 {
					a.Key = ""
					a.Value = slog.AnyValue(nil)
				}
				return a
			}

			logOptions.ReplaceAttr = logReplace
		}

		logHandler := slog.NewTextHandler(os.Stderr, logOptions)

		// https://github.com/cyrusaf/ctxlog/pull/1
		// log := slog.New(ctxlog.NewHandler(logHandler))
		log := slog.New(logHandler)

		slog.SetDefault(log)

		rootLogger = log

	})

	return rootLogger
}

type loggerKey struct{}

// NewContext adds the logger to the context.
func NewContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// FromContext retrieves a logger from the context. If there is none,
// it returns the default logger.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return Setup()
}
