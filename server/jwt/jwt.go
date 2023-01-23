package jwt

import (
	"fmt"
	"log"
	"time"

	gjwt "github.com/golang-jwt/jwt/v4"

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

	mySigningKey := []byte(key)

	publish := []string{}
	subscribe := []string{}
	expireAt := time.Now().Add(365 * 24 * time.Hour)
	notBefore := time.Now().Add(-30 * time.Second)
	// log.Printf("not before: %s", notBefore)

	depEnv := api.DeployUndefined
	var err error

	switch subject {
	case "monitor-api-dev.ntppool.net":
		depEnv = api.DeployDevel
	case "mqtt-admin.mon.ntppool.dev":
		// for admin cli tool

	default:
		depEnv, err = api.GetDeploymentEnvironmentFromName(subject)
		if err != nil {
			return "", err
		}
	}

	topics := mqttcm.NewTopics(depEnv)

	if admin {
		expireAt = time.Now().Add(168 * time.Hour)
		subscribe = append(subscribe, "#", "/#", "devel/#")
		publish = append(publish, "#", "/#")
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

	log.Printf("subscribe topics: %+v", subscribe)
	log.Printf("publish   topics: %+v", publish)

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

	log.Printf("claims: %+v", claims)

	token := gjwt.NewWithClaims(gjwt.SigningMethodHS384, claims)
	ss, err := token.SignedString(mySigningKey)
	if err != nil {
		return "", err
	}
	return ss, nil
}
