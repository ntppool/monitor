package jwt

import (
	"context"
	"fmt"
	"time"

	gjwt "github.com/golang-jwt/jwt/v4"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/mqttcm"
)

type KeyType uint8

const (
	KeyTypeStandard KeyType = iota
	KeyTypeServer
	KeyTypeAdmin
	KeyTypeExporter
)

type MosquittoClaims struct {
	// "subs": ["/+/topic", "/abc/#"],
	Subscribe []string `json:"subs"`
	Publish   []string `json:"publ"`
	gjwt.RegisteredClaims
}

func GetToken(ctx context.Context, key, subject string, keyType KeyType) (string, error) {

	log := logger.Setup()

	mySigningKey := []byte(key)

	publish := []string{}
	subscribe := []string{}
	expireAt := time.Now().Add(6 * time.Hour)
	if keyType == KeyTypeExporter {
		expireAt = time.Now().Add(24 * 365 * 3 * time.Hour)
	}
	notBefore := time.Now().Add(-30 * time.Second)
	// log.Printf("not before: %s", notBefore)

	depEnv := api.DeployUndefined
	var err error

	switch subject {
	case "monitor-api-dev.ntppool.net":
		depEnv = api.DeployDevel
	case "mqtt-admin.mon.ntppool.dev":
		// for admin cli tool
		depEnv = api.DeployDevel
	case "exporter.mqtt.ntppool.net":
		// for the prometheus exporter
		depEnv = api.DeployDevel

	default:
		depEnv, err = api.GetDeploymentEnvironmentFromName(subject)
		if err != nil {
			return "", err
		}
	}

	topics := mqttcm.NewTopics(depEnv)

	switch keyType {
	case KeyTypeAdmin:
		expireAt = time.Now().Add(5 * time.Minute) // server code generates a new one as needed
		// subscribe = append(subscribe, "#", "/#", "devel/#")
		// publish = append(publish, "#", "/#")
		subscribe = append(subscribe, "/"+depEnv.String()+"/#")
		// subscribe = append(subscribe, "#")

		publish = append(publish, "/"+depEnv.String()+"/#")

	case KeyTypeExporter:
		subscribe = append(subscribe,
			"$SYS/#",
		)

	case KeyTypeServer:
		subscribe = append(subscribe, fmt.Sprintf("/%s/#", depEnv))
		publish = append(publish, fmt.Sprintf("/%s/#", depEnv))

	case KeyTypeStandard:
		subscribe = append(subscribe,
			topics.RequestSubscription(subject),
		)
		publish = append(publish,
			// %u and %c aren't supported
			topics.Status(subject),
			topics.StatusAPITest(subject),

			fmt.Sprintf("/%s/monitors/data/%s/+", depEnv, subject),
			fmt.Sprintf("/%s/monitors/data/%s/+/+", depEnv, subject),
		)
	}

	// log.DebugContext(ctx, "jwt setup", "subscribe", subscribe, "publish", publish)

	claims := MosquittoClaims{
		subscribe,
		publish,
		gjwt.RegisteredClaims{
			ExpiresAt: gjwt.NewNumericDate(expireAt),
			IssuedAt:  gjwt.NewNumericDate(time.Now()),
			NotBefore: gjwt.NewNumericDate(notBefore),
			Subject:   subject,

			// mosquitto-jwt-auth doesn't know how to use these
			// Issuer:    "ntppool-monitor",
			// Audience:  []string{"mqtt.ntppool.net"},
		},
	}

	log.DebugContext(ctx, "jwt claims", "claims", claims)

	token := gjwt.NewWithClaims(gjwt.SigningMethodHS384, claims)
	ss, err := token.SignedString(mySigningKey)
	if err != nil {
		return "", err
	}
	return ss, nil
}
