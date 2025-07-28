package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/eclipse/paho.golang/paho"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/client/config/checkconfig"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	"go.ntppool.org/monitor/mqttcm"
)

type apiCmd struct {
	Ok apiOkCmd `cmd:"" help:"Check API connection"`
}

type apiOkCmd struct{}

func (cmd *apiOkCmd) Run(ctx context.Context, cli *ClientCmd) error {
	log := logger.FromContext(ctx)

	timeout := time.Second * 40
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.InfoContext(ctx, "Running API diagnostics", "env", cli.DeployEnv)

	// Step 1: Check if we have an API key
	apiKey := cli.Config.APIKey()
	if apiKey == "" {
		log.WarnContext(ctx, "No API key found - run 'ntppool-agent setup' to configure")
		return fmt.Errorf("no API key configured")
	}
	log.InfoContext(ctx, "✓ API key found")

	// Step 2: Test HTTP API endpoint with API key
	log.InfoContext(ctx, "Testing HTTP API endpoint...")
	httpOK, err := testHTTPAPI(ctx, cli)
	if err != nil {
		log.ErrorContext(ctx, "✗ HTTP API test failed", "err", err)
		if errors.Is(err, config.ErrAuthorization) {
			log.ErrorContext(ctx, "API key is invalid or unauthorized - run 'ntppool-agent setup' to reconfigure")
			return err
		}
	} else if httpOK {
		log.InfoContext(ctx, "✓ HTTP API test successful")
	}

	// Step 3: Check certificate status
	haveCert := cli.Config.HaveCertificate()
	if !haveCert {
		log.WarnContext(ctx, "No certificates found")
		log.InfoContext(ctx, "Certificates are issued after monitor is activated in the web interface")

		// Try to load certificates if monitor might be active
		if httpOK {
			log.InfoContext(ctx, "Attempting to request certificates...")
			_, err := cli.Config.LoadAPIAppConfigWithCertificateRequest(ctx)
			if err != nil {
				log.DebugContext(ctx, "Could not load certificates", "err", err)
			} else if cli.Config.HaveCertificate() {
				log.InfoContext(ctx, "✓ Certificates loaded successfully")
				haveCert = true
			}
		}
	}

	if haveCert {
		_, certNotAfter, remaining, err := cli.Config.CertificateDates()
		if err != nil {
			log.ErrorContext(ctx, "Could not get certificate dates", "err", err)
		} else {
			log.InfoContext(ctx, "✓ Certificate found",
				"expires", certNotAfter.Format(time.RFC3339),
				"remaining", remaining.Round(time.Hour))

			if remaining < 24*time.Hour {
				log.WarnContext(ctx, "Certificate expires soon!")
			} else {
				tracingShutdown, err := InitTracing(ctx, cli.DeployEnv, cli.Config)
				if err != nil {
					log.WarnContext(ctx, "tracing initialization failed", "err", err)
				} else {
					defer func() {
						shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
						defer cancel()
						if err := tracingShutdown(shutdownCtx); err != nil {
							log.WarnContext(ctx, "tracing shutdown error", "err", err)
						}
					}()
				}
			}
		}
	}

	// Step 4: Check monitor status
	ipv4Status := cli.Config.IPv4()
	ipv6Status := cli.Config.IPv6()

	log.InfoContext(ctx, "Monitor status:")
	if ipv4Status.IP != nil {
		log.InfoContext(ctx, "  IPv4", "ip", ipv4Status.IP.String(), "status", ipv4Status.Status)
	} else {
		log.InfoContext(ctx, "  IPv4: not configured")
	}

	if ipv6Status.IP != nil {
		log.InfoContext(ctx, "  IPv6", "ip", ipv6Status.IP.String(), "status", ipv6Status.Status)
	} else {
		log.InfoContext(ctx, "  IPv6: not configured")
	}

	monitorActive := ipv4Status.IsLive() || ipv6Status.IsLive()
	if !monitorActive {
		log.WarnContext(ctx, "Monitor is not active (status must be 'active' or 'testing')")
		if !haveCert {
			return nil // No point testing gRPC without certificates
		}
	}

	// Step 5: Test gRPC API if we have certificates
	if haveCert {
		log.InfoContext(ctx, "Testing gRPC API endpoint...")
		err := testGRPCAPI(ctx, cli, log)
		if err != nil {
			log.ErrorContext(ctx, "✗ gRPC API test failed", "err", err)
		} else {
			log.InfoContext(ctx, "✓ gRPC API test successful")
		}
	}

	log.InfoContext(ctx, "API diagnostics complete")

	return nil
}

// testHTTPAPI tests the HTTP API endpoint using the API key
func testHTTPAPI(ctx context.Context, cli *ClientCmd) (bool, error) {
	// Just try to load the config - this tests the HTTP API
	changed, err := cli.Config.LoadAPIAppConfig(ctx)
	if err != nil {
		return false, err
	}
	// Return true if we successfully loaded (changed or not)
	_ = changed
	return true, nil
}

// testGRPCAPI tests the gRPC API endpoint using certificates
func testGRPCAPI(ctx context.Context, cli *ClientCmd, log *slog.Logger) error {
	ctx, span := tracing.Start(ctx, "grpc-api-test")
	defer span.End()

	// Setup gRPC client
	ctx, apiC, err := api.Client(ctx, cli.Config.TLSName(), cli.Config)
	if err != nil {
		return fmt.Errorf("could not setup gRPC client: %w", err)
	}

	// Try to get config via gRPC
	cfgresp, err := apiC.GetConfig(ctx, connect.NewRequest(&apiv2.GetConfigRequest{}))
	if err != nil {
		if conerr, ok := err.(*connect.Error); ok {
			switch conerr.Code() {
			case connect.CodePermissionDenied:
				return fmt.Errorf("permission denied: %w", err)
			case connect.CodeUnauthenticated:
				return fmt.Errorf("authentication failed: %w", err)
			default:
				return fmt.Errorf("gRPC error (code %v): %w", conerr.Code(), err)
			}
		}
		return fmt.Errorf("could not get config: %w", err)
	}

	if cfgresp == nil || cfgresp.Msg == nil {
		return fmt.Errorf("received empty configuration response")
	}

	cfg := cfgresp.Msg

	// Log some basic config info
	if cfg.Samples > 0 {
		log.InfoContext(ctx, "  Config received", "samples", cfg.Samples)
	}

	// Test MQTT if configured
	if cfg.MqttConfig != nil && len(cfg.MqttConfig.Host) > 0 {
		log.InfoContext(ctx, "  MQTT configured", "host", cfg.MqttConfig.Host)

		// Test MQTT connection and publish status message
		conf := checkconfig.NewConfigger(nil)
		conf.SetConfigFromApi(cfg)

		mq, err := mqttcm.Setup(ctx, cli.Config.TLSName(), "", []string{}, nil, conf, cli.Config)
		if err != nil {
			log.WarnContext(ctx, "  MQTT setup failed", "err", err)
		} else {
			defer func() {
				if err := mq.Disconnect(ctx); err != nil {
					log.WarnContext(ctx, "Failed to disconnect MQTT", "err", err)
				}
			}()

			mqctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			err := mq.AwaitConnection(mqctx)
			if err != nil {
				log.WarnContext(ctx, "  MQTT connection failed", "err", err)
			} else {
				log.InfoContext(ctx, "  ✓ MQTT connection successful")

				// Publish test message to StatusAPITest topic
				msg := []byte(fmt.Sprintf("API test - %s", time.Now().Format(time.RFC3339)))
				topics := mqttcm.NewTopics(cli.Config.Env())

				_, err = mq.Publish(ctx, &paho.Publish{
					QoS:     1,
					Topic:   topics.StatusAPITest(cli.Config.TLSName()),
					Payload: msg,
					Retain:  false,
				})
				if err != nil {
					log.WarnContext(ctx, "  MQTT publish failed", "err", err)
				} else {
					log.InfoContext(ctx, "  ✓ MQTT test message published")
				}
			}
		}
	}

	return nil
}
