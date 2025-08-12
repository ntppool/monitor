module go.ntppool.org/monitor

go 1.24

// replace github.com/eclipse/paho.golang => /Users/ask/src/paho.golang/
// replace go.ntppool.org/common => /Users/ask/src/go/ntp/common/

tool (
	connectrpc.com/connect/cmd/protoc-gen-connect-go
	github.com/bufbuild/buf/cmd/buf
	github.com/dmarkham/enumer
	github.com/hexdigest/gowrap/cmd/gowrap
	google.golang.org/protobuf/cmd/protoc-gen-go
)

require (
	connectrpc.com/connect v1.18.1
	connectrpc.com/otelconnect v0.7.2
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/MicahParks/keyfunc/v3 v3.4.0
	github.com/abh/certman v0.4.0
	github.com/alecthomas/kong v1.12.1
	github.com/beevik/ntp v1.4.3
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/eclipse/paho.golang v0.22.0
	github.com/evanphx/json-patch v0.5.2
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-sql-driver/mysql v1.9.3
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/hashicorp/vault/api v1.20.0
	github.com/labstack/echo/v4 v4.13.4
	github.com/oklog/ulid/v2 v2.1.1
	github.com/prometheus/client_golang v1.23.0
	github.com/samber/slog-echo v1.16.1
	github.com/stretchr/testify v1.10.0
	github.com/twitchtv/twirp v8.1.3+incompatible
	go.ntppool.org/common v0.5.2-0.20250803192035-28d05d1d0ee6
	go.ntppool.org/pingtrace v0.0.4
	go.ntppool.org/vault-token-manager v0.0.1
	go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho v0.62.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.62.0
	go.opentelemetry.io/otel v1.37.0
	go.opentelemetry.io/otel/metric v1.37.0
	go.opentelemetry.io/otel/trace v1.37.0
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba
	golang.org/x/exp v0.0.0-20250718183923-645b1fa84792
	golang.org/x/sync v0.16.0
	google.golang.org/protobuf v1.36.6
	inet.af/netaddr v0.0.0-20230525184311-b8eac61e914a
)

require (
	buf.build/gen/go/bufbuild/bufplugin/protocolbuffers/go v1.36.6-20250121211742-6d880cc6cc8d.1 // indirect
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.6-20250613105001-9f2d3c737feb.1 // indirect
	buf.build/gen/go/bufbuild/registry/connectrpc/go v1.18.1-20250606164443-9d1800bf4ccc.1 // indirect
	buf.build/gen/go/bufbuild/registry/protocolbuffers/go v1.36.6-20250606164443-9d1800bf4ccc.1 // indirect
	buf.build/gen/go/pluginrpc/pluginrpc/protocolbuffers/go v1.36.6-20241007202033-cf42259fcbfc.1 // indirect
	buf.build/go/app v0.1.0 // indirect
	buf.build/go/bufplugin v0.9.0 // indirect
	buf.build/go/interrupt v1.1.0 // indirect
	buf.build/go/protovalidate v0.13.1 // indirect
	buf.build/go/protoyaml v0.6.0 // indirect
	buf.build/go/spdx v0.2.0 // indirect
	buf.build/go/standard v0.1.0 // indirect
	cel.dev/expr v0.24.0 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/Masterminds/sprig/v3 v3.2.2 // indirect
	github.com/MicahParks/jwkset v0.9.6 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bufbuild/buf v1.55.1 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/bufbuild/protoplugin v0.0.0-20250218205857-750e09ce93e1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.16.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/dmarkham/enumer v1.5.11 // indirect
	github.com/docker/cli v28.2.2+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker v28.2.2+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.9.3 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-chi/chi/v5 v5.2.1 // indirect
	github.com/go-jose/go-jose/v4 v4.1.2 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gofrs/flock v0.12.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/cel-go v0.25.0 // indirect
	github.com/google/go-containerregistry v0.20.6 // indirect
	github.com/google/pprof v0.0.0-20250607225305-033d6d78b36a // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-7 // indirect
	github.com/hexdigest/gowrap v1.4.2 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jdx/go-netrc v1.0.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/copystructure v1.1.2 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.1 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo/v2 v2.23.4 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pascaldekloe/name v1.0.1 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.52.0 // indirect
	github.com/remychantenay/slog-otel v1.3.4 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/samber/lo v1.51.0 // indirect
	github.com/samber/slog-common v0.19.0 // indirect
	github.com/samber/slog-multi v1.4.1 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/segmentio/encoding v0.5.1 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/cobra v1.9.1 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/tetratelabs/wazero v1.9.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/vbatts/tar-split v0.12.1 // indirect
	go.lsp.dev/jsonrpc2 v0.10.0 // indirect
	go.lsp.dev/pkg v0.0.0-20210717090340-384b27a52fb2 // indirect
	go.lsp.dev/protocol v0.12.0 // indirect
	go.lsp.dev/uri v0.3.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelslog v0.12.0 // indirect
	go.opentelemetry.io/contrib/bridges/prometheus v0.62.0 // indirect
	go.opentelemetry.io/contrib/exporters/autoexport v0.62.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.13.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.13.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.37.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.37.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.37.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.37.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.37.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.59.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.13.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.37.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.37.0 // indirect
	go.opentelemetry.io/otel/log v0.13.0 // indirect
	go.opentelemetry.io/otel/sdk v1.37.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.13.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.37.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.1 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/mock v0.5.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	go4.org/intern v0.0.0-20230525184215-6c62f75575cb // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/term v0.34.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.35.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250728155136-f173205681a0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250728155136-f173205681a0 // indirect
	google.golang.org/grpc v1.74.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	pluginrpc.com/pluginrpc v0.5.0 // indirect
)

replace go.opentelemetry.io/otel/exporters/prometheus v0.59.1 => go.opentelemetry.io/otel/exporters/prometheus v0.59.0
