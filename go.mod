module go.ntppool.org/monitor

go 1.17

//replace github.com/dyson/certman => github.com/abh/certman v0.3.1
replace github.com/dyson/certman => /Users/ask/go/src/github.com/abh/certman

require (
	github.com/beevik/ntp v0.3.0
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dyson/certman v0.2.1
	github.com/kr/pretty v0.3.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/twitchtv/twirp v8.1.0+incompatible
	google.golang.org/protobuf v1.27.1
	inet.af/netaddr v0.0.0-20211027220019-c74959edd3b6
)

require (
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pkg/errors v0.8.1 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go4.org/intern v0.0.0-20211027215823-ae77deb06f29 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20211027215541-db492cf91b37 // indirect
	golang.org/x/net v0.0.0-20211123203042-d83791d6bcd9 // indirect
	golang.org/x/sys v0.0.0-20211123173158-ef496fb156ab // indirect
)
