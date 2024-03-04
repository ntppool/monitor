module go.ntppool.org/monitor

go 1.21.6

toolchain go1.22.0

require (
	connectrpc.com/connect v1.15.0
	connectrpc.com/otelconnect v0.7.0
	github.com/abh/certman v0.4.0
	github.com/beevik/ntp v1.3.1
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/cristalhq/aconfig v0.18.5
	github.com/cristalhq/aconfig/aconfigdotenv v0.17.1
	github.com/cristalhq/aconfig/aconfigyaml v0.17.1
	github.com/eclipse/paho.golang v0.21.0
	github.com/evanphx/json-patch v0.5.2
	github.com/go-sql-driver/mysql v1.7.1
	github.com/golang-jwt/jwt/v4 v4.5.0
	github.com/gorilla/handlers v1.5.2
	github.com/hashicorp/vault/api v1.12.0
	github.com/hashicorp/vault/api/auth/approle v0.6.0
	github.com/labstack/echo/v4 v4.11.4
	github.com/oklog/ulid/v2 v2.1.0
	github.com/prometheus/client_golang v1.19.0
	github.com/prometheus/common v0.49.0
	github.com/samber/slog-echo v1.12.2
	github.com/spf13/cobra v1.8.0
	github.com/twitchtv/twirp v8.1.3+incompatible
	go.ntppool.org/common v0.2.7
	go.ntppool.org/pingtrace v0.0.4
	go.ntppool.org/vault-token-manager v0.0.1
	go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho v0.49.0
	go.opentelemetry.io/otel v1.24.0
	go.opentelemetry.io/otel/trace v1.24.0
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba
	golang.org/x/exp v0.0.0-20240222234643-814bf88cf225
	golang.org/x/mod v0.15.0
	golang.org/x/sync v0.6.0
	google.golang.org/protobuf v1.32.0
	inet.af/netaddr v0.0.0-20230525184311-b8eac61e914a
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.2 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.5 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.8 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.6 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/remychantenay/slog-otel v1.2.4 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/samber/lo v1.39.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.24.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.24.0 // indirect
	go.opentelemetry.io/otel/sdk v1.24.0 // indirect
	go.opentelemetry.io/proto/otlp v1.1.0 // indirect
	go4.org/intern v0.0.0-20230525184215-6c62f75575cb // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	golang.org/x/crypto v0.20.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240228224816-df926f6c8641 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240228224816-df926f6c8641 // indirect
	google.golang.org/grpc v1.62.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
