package jwt

import (
	"fmt"
	"time"

	gjwt "github.com/golang-jwt/jwt/v4"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/mqttcm"
)

type MosquittoClaims struct {
	// "subs": ["/+/topic", "/abc/#"],
	Subscribe []string `json:"subs"`
	Publish   []string `json:"publ"`
	gjwt.RegisteredClaims
}

func GetToken(key, subject string, admin bool) (string, error) {

	log := logger.Setup()

	mySigningKey := []byte(key)

	publish := []string{}
	subscribe := []string{}
	expireAt := time.Now().Add(6 * time.Hour)
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

	default:
		depEnv, err = api.GetDeploymentEnvironmentFromName(subject)
		if err != nil {
			return "", err
		}
	}

	topics := mqttcm.NewTopics(depEnv)

	if admin {
		expireAt = time.Now().Add(5 * time.Minute) // server code generates a new one as needed
		// subscribe = append(subscribe, "#", "/#", "devel/#")
		// publish = append(publish, "#", "/#")
		subscribe = append(subscribe, "/"+depEnv.String()+"/#")
		// subscribe = append(subscribe, "#")

		publish = append(publish, "/"+depEnv.String()+"/#")
	} else {
		subscribe = append(subscribe,
			// "#", "/#",
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

	log.Debug("jwt setup", "subscribe", subscribe)
	log.Debug("jwt setup", "publish", publish)

	claims := MosquittoClaims{
		subscribe,
		publish,
		gjwt.RegisteredClaims{
			ExpiresAt: gjwt.NewNumericDate(expireAt),
			// IssuedAt:  gjwt.NewNumericDate(notBefore),
			NotBefore: gjwt.NewNumericDate(notBefore),
			Subject:   subject,
			// Issuer:  "ntppool-monitor",
			// Audience:  []string{"mqtt.ntppool.net"},
			// ID:        "1",
		},
	}

	log.Debug("jwt claims", "claims", claims)

	token := gjwt.NewWithClaims(gjwt.SigningMethodHS384, claims)
	ss, err := token.SignedString(mySigningKey)
	if err != nil {
		return "", err
	}
	return ss, nil
}
