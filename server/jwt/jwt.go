package jwt

import (
	"fmt"
	"log"
	"time"

	gjwt "github.com/golang-jwt/jwt/v4"

	"go.ntppool.org/monitor/api"
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
	expireAt := time.Now().Add(24 * time.Hour)
	notBefore := time.Now().Add(-30 * time.Second)
	log.Printf("not before: %s", notBefore)

	depEnv, err := api.GetDeploymentEnvironment(subject)
	if err != nil {
		return "", err
	}

	if admin {
		expireAt = time.Now().Add(168 * time.Hour)
		subscribe = append(subscribe, "#", "/#", "/devel/#", "/devel/monitors/monitor-status")
		publish = append(publish, "#", "/#")
	} else {
		subscribe = append(subscribe,
			// "#", "/#",
			fmt.Sprintf("/%s/monitors/requests/%s", depEnv, subject),
		)

		publish = append(publish,
			// %u and %c aren't supported
			fmt.Sprintf("/%s/monitors/status/%s/+", depEnv, subject),
			fmt.Sprintf("/%s/monitors/data/%s", depEnv, subject),
			fmt.Sprintf("/%s/monitors/data/%s/+", depEnv, subject),
		)
	}

	claims := MosquittoClaims{
		subscribe,
		publish,
		gjwt.RegisteredClaims{
			ExpiresAt: gjwt.NewNumericDate(expireAt),
			// IssuedAt:  gjwt.NewNumericDate(notBefore),
			// NotBefore: gjwt.NewNumericDate(notBefore),
			// Issuer:  "ntppool-monitor",
			Subject: subject,
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
