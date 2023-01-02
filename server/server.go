package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twitchtv/twirp"
	otrace "go.opentelemetry.io/otel/trace"

	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/ntpdb"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/metrics"
	twirpmetrics "go.ntppool.org/monitor/server/metrics/twirp"
	twirptrace "go.ntppool.org/monitor/server/twirptrace"
	"go.ntppool.org/monitor/server/vault"
)

type Server struct {
	cfg    *Config
	tokens *vault.TokenManager
	m      *metrics.Metrics
	tracer otrace.Tracer
	db     *ntpdb.Queries
	dbconn *sql.DB
}

type Config struct {
	DeploymentEnv string
	Listen        string
	JWTKey        string
	CertProvider  apitls.CertificateProvider
}

func NewServer(cfg Config, dbconn *sql.DB) (*Server, error) {
	db := ntpdb.New(dbconn)

	tm, err := vault.New("monitor-tokens", cfg.DeploymentEnv)
	if err != nil {
		return nil, err
	}

	metrics := metrics.New()

	srv := &Server{
		cfg:    &cfg,
		db:     db,
		dbconn: dbconn,
		tokens: tm,
		m:      metrics,
	}

	err = srv.initTracer(cfg.DeploymentEnv)
	if err != nil {
		return nil, err
	}

	srv.tracer = srv.NewTracer()

	return srv, nil
}

func (srv *Server) Run() error {

	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{})
	// logrusEntry := logrus.NewEntry(logger)

	// todo: make this function actually quit on shutdown...
	// defer srv.tracer.Close() // flush buffered spans

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

	hooks := twirp.ChainHooks(
		twirptrace.NewOpenTracingHooks(
			srv.tracer,
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
		srv.certificateMiddleware(
			WithUserAgent(
				twirptrace.WithTraceContext(
					twirpHandler, srv.tracer,
				),
			),
		),
	)

	logger.Infof("starting server")

	metricsServer := &http.Server{
		Addr:    ":9000",
		Handler: srv.m.Handler(),
	}

	go func() {
		log.Printf("metrics server: %s", metricsServer.ListenAndServe())
	}()

	server := &http.Server{
		Addr: ":8000",

		TLSConfig: tlsConfig,
		Handler:   mux,

		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       240 * time.Second,
	}

	return server.ListenAndServeTLS("", "")
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

			log.Printf("method=%s cn=%s TraceID=%s", d.MethodName, d.CN, d.Span.SpanContext().TraceID())
			return ctx, nil
		},
		Error: func(ctx context.Context, twerr twirp.Error) context.Context {
			d := logDataFn(ctx)
			log.Printf("method=%s cn=%s TraceID=%s error=%q message=%q",
				d.MethodName, d.CN, d.Span.SpanContext().TraceID(),
				string(twerr.Code()), twerr.Msg())
			return ctx
		},
		ResponseSent: func(ctx context.Context) {
			d := logDataFn(ctx)
			requestStart, _ := ctx.Value(sctx.RequestStartKey).(time.Time)
			duration := time.Since(requestStart)

			log.Printf("method=%s cn=%s TraceID=%s duration=%s",
				d.MethodName, d.CN, d.Span.SpanContext().TraceID(), duration)
		},
	}
}
