package config

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/common/logger"
)

// Manager handles AppConfig hot reloading and certificate management
func (ac *appConfig) Manager(ctx context.Context, promreg prometheus.Registerer) error {
	log := logger.FromContext(ctx).WithGroup("appconfig-manager")

	promGauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "ssl_earliest_cert_expiry",
		Help: "TLS expiration time",
	}, func() float64 {
		_, notAfter, _, err := ac.CertificateDates()
		if err != nil {
			log.Error("could not get certificate notAfter date", "err", err)
			return 0
		}
		return float64(notAfter.Unix())
	})
	promreg.MustRegister(promGauge)
	// Create file watcher for state.json
	stateFilePath := ac.stateFilePrefix(stateFile)
	stateDir, stateFileName := filepath.Dir(stateFilePath), filepath.Base(stateFilePath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WarnContext(ctx, "failed to create file watcher, falling back to timer-only reloading", "err", err)
		watcher = nil
	} else {
		err = watcher.Add(stateDir)
		if err != nil {
			log.WarnContext(ctx, "failed to watch state file directory, falling back to timer-only reloading", "dir", stateDir, "err", err)
			watcher.Close()
			watcher = nil
		} else {
			log.InfoContext(ctx, "watching state file directory for changes", "dir", stateDir, "file", stateFileName)
		}
	}

	// Hot reloading goroutine
	go func() {
		defer func() {
			if watcher != nil {
				watcher.Close()
			}
		}()

		// Default reload interval
		const defaultReloadInterval = 5 * time.Minute
		const errorRetryInterval = 2 * time.Minute
		const debounceInterval = 100 * time.Millisecond

		// Track previous protocol states for change detection
		var prevIPv4Live, prevIPv6Live bool

		// Debounce timer for rapid file changes
		var debounceTimer *time.Timer

		// Create timer for periodic reloads
		timer := time.NewTimer(defaultReloadInterval)
		defer timer.Stop()

		for {
			var reloadTriggered bool
			var nextCheck time.Duration

			if watcher != nil {
				// Add debounce timer channel if available
				var debounceC <-chan time.Time
				if debounceTimer != nil {
					debounceC = debounceTimer.C
				}

				select {
				case <-debounceC:
					log.DebugContext(ctx, "debounce timer fired, triggering reload")
					reloadTriggered = true
					debounceTimer = nil

				case event, ok := <-watcher.Events:
					if !ok {
						log.WarnContext(ctx, "file watcher events channel closed")
						watcher = nil
						reloadTriggered = true
					} else {
						// Check for various file operations that might indicate a change
						// Some systems use Create for atomic renames, others use Write
						baseName := filepath.Base(event.Name)
						if baseName == stateFileName || baseName == stateFileName+".tmp" {
							if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
								log.InfoContext(ctx, "state file changed, triggering reload",
									"event", event.String(),
									"file", baseName)
								// Cancel any pending debounce timer
								if debounceTimer != nil {
									debounceTimer.Stop()
								}
								// Set a short debounce timer to handle rapid successive events
								debounceTimer = time.NewTimer(debounceInterval)
								// Continue to wait for debounce timer
								continue
							}
						}
					}

				case err, ok := <-watcher.Errors:
					if !ok {
						log.WarnContext(ctx, "file watcher error channel closed")
						watcher = nil
					} else {
						log.WarnContext(ctx, "file watcher error", "err", err)
					}
					reloadTriggered = true

				case <-timer.C:
					log.DebugContext(ctx, "timer triggered reload")
					reloadTriggered = true

				case <-ctx.Done():
					log.InfoContext(ctx, "AppConfig hot reloader shutting down")
					return
				}
			} else {
				// No watcher, only use timer and context
				select {
				case <-timer.C:
					log.DebugContext(ctx, "timer triggered reload")
					reloadTriggered = true

				case <-ctx.Done():
					log.InfoContext(ctx, "AppConfig hot reloader shutting down")
					return
				}
			}

			if reloadTriggered {
				// Reload configuration from disk (including state.json with API key) and API
				err := ac.load(ctx)

				if err != nil {
					log.WarnContext(ctx, "failed to reload AppConfig", "err", err)
					nextCheck = errorRetryInterval
				} else {
					log.DebugContext(ctx, "AppConfig reloaded successfully")
					nextCheck = defaultReloadInterval

					// Check for protocol status changes
					currentIPv4Live := ac.IPv4().IsLive()
					currentIPv6Live := ac.IPv6().IsLive()

					if currentIPv4Live != prevIPv4Live {
						log.InfoContext(ctx, "IPv4 protocol status changed",
							"previous", prevIPv4Live, "current", currentIPv4Live,
							"status", ac.IPv4().Status, "ip", ac.IPv4().IP)
						prevIPv4Live = currentIPv4Live
					}

					if currentIPv6Live != prevIPv6Live {
						log.InfoContext(ctx, "IPv6 protocol status changed",
							"previous", prevIPv6Live, "current", currentIPv6Live,
							"status", ac.IPv6().Status, "ip", ac.IPv6().IP)
						prevIPv6Live = currentIPv6Live
					}
				}

				// Ensure we don't check too frequently (minimum 1 minute) or too infrequently (maximum 1 hour)
				if nextCheck < 1*time.Minute {
					nextCheck = 1 * time.Minute
				} else if nextCheck > 1*time.Hour {
					nextCheck = 1 * time.Hour
				}

				log.DebugContext(ctx, "scheduling next AppConfig reload", "duration", nextCheck)
				timer.Reset(nextCheck)
			}
		}
	}()

	return nil
}
