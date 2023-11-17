package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/twitchtv/twirp"
	otrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/ntpdb"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/metrics"
	twirpmetrics "go.ntppool.org/monitor/server/metrics/twirp"
	twirptrace "go.ntppool.org/monitor/server/twirptrace"
	vtm "go.ntppool.org/vault-token-manager"
)

type Server struct {
	ctx         context.Context
	cfg         *Config
	tokens      *vtm.TokenManager
	m           *metrics.Metrics
	db          *ntpdb.Queries
	dbconn      *sql.DB
	log         *slog.Logger
	shutdownFns []func(ctx context.Context) error
}

type Config struct {
	DeploymentEnv string
	Listen        string
	JWTKey        string
	CertProvider  apitls.CertificateProvider
}

func NewServer(ctx context.Context, log *slog.Logger, cfg Config, dbconn *sql.DB, promRegistry prometheus.Registerer) (*Server, error) {
	db := ntpdb.New(dbconn)

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

	metrics := metrics.New(promRegistry)

	srv := &Server{
		ctx:    ctx,
		cfg:    &cfg,
		log:    log,
		db:     db,
		dbconn: dbconn,
		tokens: tm,
		m:      metrics,
	}

	// capool, err := apitls.CAPool()
	// if err != nil {
	// 	return nil, err
	// }

	tpShutdownFn, err := tracing.InitTracer(ctx,
		&tracing.TracerConfig{
			ServiceName: "monitor-api",
			Environment: cfg.DeploymentEnv,
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

	srv.shutdownFns = append(srv.shutdownFns, tpShutdownFn)

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

	tlsConfig := &tls.Config{
		MinVersion:            tls.VersionTLS12,
		ClientCAs:             capool,
		ClientAuth:            tls.RequireAndVerifyClientCert,
		GetCertificate:        srv.cfg.CertProvider.GetCertificate,
		VerifyPeerCertificate: srv.verifyClient,
	}

	tracer := tracing.Tracer()

	hooks := twirp.ChainHooks(
		twirptrace.NewOpenTracingHooks(
			tracer,
			twirptrace.WithTags(twirptrace.TraceTag{Key: "ottwirp", Value: true}),
			twirptrace.IncludeClientErrors(true),
			twirptrace.WithContextTags(func(ctx context.Context) (context.Context, []twirptrace.TraceTag) {
				mon, ctx, err := srv.getMonitor(ctx)
				if err != nil {
					return ctx, nil
				}
				return ctx, []twirptrace.TraceTag{
					{
						Key:   "monitor_id",
						Value: mon.ID,
					},
					{
						Key:   "monitor_name",
						Value: mon.TlsName.String,
					},
					{
						Key:   "monitor_account",
						Value: mon.AccountID.Int32,
					},
				}
			}),
		),
		NewLoggingServerHooks(),
		twirpmetrics.NewServerHooks(srv.m.Registry()),
	)

	twirpHandler := pb.NewMonitorServer(srv,
		twirp.WithServerPathPrefix("/api/v1"),
		twirp.WithServerHooks(hooks),
	)

	mux := http.NewServeMux()
	mux.Handle(twirpHandler.PathPrefix(),
		twirptrace.WithTraceContext(
			srv.certificateMiddleware(
				WithUserAgent(
					twirpHandler,
				),
			),
			tracer,
		),
	)

	listen := srv.cfg.Listen

	srv.log.Info("starting server", "port", listen)

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

	srv.log.Info("Starting gRPC server", "port", listen)

	g.Go(func() error {
		err := server.ListenAndServeTLS("", "")
		if err != nil {
			return fmt.Errorf("server listen: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		log.Info("shutting down twirp server")
		server.Shutdown(ctx)
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

func NewLoggingServerHooks() *twirp.ServerHooks {

	type logData struct {
		CN         string
		MethodName string
		Span       otrace.Span
	}

	logDataFn := func(ctx context.Context) *logData {
		method, _ := twirp.MethodName(ctx)
		span := otrace.SpanFromContext(ctx)
		cn := getCertificateName(ctx)

		return &logData{
			CN:         cn,
			MethodName: method,
			Span:       span,
		}
	}

	return &twirp.ServerHooks{
		RequestRouted: func(ctx context.Context) (context.Context, error) {
			d := logDataFn(ctx)

			ctx = context.WithValue(ctx, sctx.RequestStartKey, time.Now())

			log := logger.Setup()
			log = log.With("request", d.MethodName, "cn", d.CN, "TraceID", d.Span.SpanContext().TraceID())
			ctx = logger.NewContext(ctx, log)

			return ctx, nil
		},
		Error: func(ctx context.Context, twerr twirp.Error) context.Context {
			log := logger.FromContext(ctx)
			log.Error("request error", "error", string(twerr.Code()), "msg", twerr.Msg())
			return ctx
		},
		ResponseSent: func(ctx context.Context) {
			requestStart, _ := ctx.Value(sctx.RequestStartKey).(time.Time)
			duration := time.Since(requestStart)

			log := logger.FromContext(ctx)
			log.Debug("completed", "duration", duration)
		},
	}
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
