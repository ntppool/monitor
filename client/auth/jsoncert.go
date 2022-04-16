package auth

// import (
// 	"crypto/x509"
// 	"encoding/json"
// )

// func (ca *ClientAuth) MarshalJSON() ([]byte, error) {
// 	type caAlias *ClientAuth

// 	if ca.cert != nil {
// 		ca.CertRaw = ca.cert.Raw
// 	}

// 	tmp := (caAlias)(ca)
// 	return json.Marshal(&tmp)
// }

// func (ca *ClientAuth) UnmarshalJSON(data []byte) error {

// 	var err error

// 	if err = json.Unmarshal(data, &ca); err != nil {
// 		return err
// 	}

// 	if len(ca.CertRaw) > 0 {
// 		ca.cert, err = x509.ParseCertificate(ca.CertRaw)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }
