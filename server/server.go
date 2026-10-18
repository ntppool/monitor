package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/otelconnect"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	apitls "go.ntppool.org/monitor/api/tls"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"
	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/server/metrics"
	vtm "go.ntppool.org/vault-token-manager"
)

type Server struct {
	ctx         context.Context
	cfg         *Config
	tokens      *vtm.TokenManager
	m           *metrics.Metrics
	meters      *sdkmetric.MeterProvider
	db          ntpdb.QuerierTx
	dbconn      *pgxpool.Pool
	jwtAuth     *JWTAuthenticator
	settings    *monitorsettings.Watcher
	clientCAs   *x509.CertPool
	shutdownFns []func(ctx context.Context) error
}

type Config struct {
	DeploymentEnv depenv.DeploymentEnvironment
	Listen        string
	JWTKey        string
	CertProvider  apitls.AuthProvider
}

func NewServer(ctx context.Context, cfg Config, dbconn *pgxpool.Pool, promRegistry prometheus.Registerer) (*Server, error) {
	db := ntpdb.NewWrappedQuerier(ntpdb.New(dbconn))
	log := logger.FromContext(ctx)

	vaultClient, err := vaultClient()
	if err != nil {
		return nil, err
	}

	tm, err := vtm.New(ctx,
		&vtm.Config{
			Vault: vaultClient,
			Path:  fmt.Sprintf("kv/data/ntppool/%s/%s", cfg.DeploymentEnv, "monitor-tokens"),
		})
	if err != nil {
		return nil, err
	}

	meters, err := metrics.NewMeterProvider(promRegistry)
	if err != nil {
		return nil, err
	}

	metrics := metrics.New(promRegistry)

	// Initialize JWT authenticator
	jwtAuth, err := NewJWTAuthenticator(ctx, cfg.DeploymentEnv)
	if err != nil {
		log.WarnContext(ctx, "failed to initialize JWT authenticator, JWT authentication will be disabled", "error", err)
		jwtAuth = nil // Continue without JWT support
	}

	settings, err := monitorsettings.NewWatcher(ctx, ntpdb.New(dbconn), log)
	if err != nil {
		return nil, fmt.Errorf("monitor settings watcher: %w", err)
	}

	srv := &Server{
		ctx:      ctx,
		cfg:      &cfg,
		db:       db,
		dbconn:   dbconn,
		tokens:   tm,
		m:        metrics,
		meters:   meters,
		jwtAuth:  jwtAuth,
		settings: settings,
	}

	// capool, err := apitls.CAPool()
	// if err != nil {
	// 	return nil, err
	// }

	tpShutdownFn, err := tracing.InitTracer(
		ctx,
		&tracing.TracerConfig{
			ServiceName: "monitor-api",
			Environment: cfg.DeploymentEnv.String(),
			// RootCAs:     capool,
		},
	)
	if err != nil {
		return nil, err
	}

	go func() {
		ctx, span := tracing.Start(context.Background(), "api-startup")
		defer span.End()
		log.InfoContext(ctx, "API startup tracing", "trace", span.SpanContext().TraceID())
	}()

	srv.shutdownFns = append(srv.shutdownFns, tpShutdownFn, meters.Shutdown)

	// Add JWT authenticator cleanup
	if jwtAuth != nil {
		srv.shutdownFns = append(srv.shutdownFns, func(ctx context.Context) error {
			jwtAuth.Close()
			return nil
		})
	}

	return srv, nil
}

func (srv *Server) Run() error {
	log := logger.FromContext(srv.ctx)

	ctx, cancel := context.WithCancel(srv.ctx)
	defer cancel()

	capool, err := apitls.CAPool()
	if err != nil {
		return err
	}

	// Store CA pool in server for certificate verification
	srv.clientCAs = capool

	tlsConfig := &tls.Config{
		MinVersion:            tls.VersionTLS12,
		ClientCAs:             capool,
		ClientAuth:            tls.RequestClientCert, // Changed from RequireAndVerifyClientCert to support JWT auth
		GetCertificate:        srv.cfg.CertProvider.GetCertificate,
		VerifyPeerCertificate: srv.verifyClient,
	}

	mux := http.NewServeMux()

	conSrv := NewConnectServer(srv)

	otelinter, err := otelconnect.NewServerInterceptor(
		otelconnect.WithTrustRemote(), // trust trace ids from the monitors
		otelconnect.WithMeterProvider(srv.meters),
	)
	if err != nil {
		return fmt.Errorf("could not setup otelconnect interceptor: %w", err)
	}
	connectServer := connect.NewServer(otelinter)
	apiv2connect.RegisterMonitorServiceHandler(connectServer, conSrv)

	// Mount registers one route per method, so it goes on its own mux to
	// keep the middleware below in front of every connect route.
	apiHandler := http.NewServeMux()
	connecthttp.Mount(apiHandler, connectServer)
	urlpath := "/" + apiv2connect.MonitorServiceName + "/"

	log.Info("setting up connectrpc", "path", urlpath)
	mux.Handle(
		urlpath,
		otelhttp.NewMiddleware("monitor-api")(
			srv.dualAuthMiddleware(
				WithLogger(
					WithUserAgent(
						apiHandler,
					),
					log,
				),
			),
		),
	)

	listen := srv.cfg.Listen

	log.Info("starting server", "port", listen)

	g, _ := errgroup.WithContext(ctx)

	server := &http.Server{
		Addr: listen,

		TLSConfig: tlsConfig,
		Handler:   mux,

		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       240 * time.Second,
	}

	g.Go(func() error {
		err := server.ListenAndServeTLS("", "")
		if err != nil {
			return fmt.Errorf("server listen: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		log.Info("shutting down server")
		if err := server.Shutdown(ctx); err != nil {
			log.Error("server shutdown error", "err", err)
		}
		errs := []error{}
		for _, fn := range srv.shutdownFns {
			err := fn(ctx)
			if err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	})

	err = g.Wait()

	return err
}

var hasOutputVaultEnvMessage bool

func vaultClient() (*vaultapi.Client, error) {
	c := vaultapi.DefaultConfig()

	if c.Address == "https://127.0.0.1:8200" {
		c.Address = "https://vault.ntppool.org"
	}

	cl, err := vaultapi.NewClient(c)
	if err != nil {
		return nil, err
	}

	// VAULT_TOKEN is read automatically from the environment if set
	// so we just try the file here
	token, err := os.ReadFile("/vault/secrets/token")
	if err == nil {
		cl.SetToken(string(token))
	} else {
		if !hasOutputVaultEnvMessage {
			hasOutputVaultEnvMessage = true
			logger.Setup().Error("could not read /vault/secrets/token, using VAULT_TOKEN", "err", err)
		}
	}

	return cl, nil
}
