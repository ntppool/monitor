package vault

import (
	"testing"
)

func TestParseSignature(t *testing.T) {
	v, err := getSignatureVersion([]byte("7-gjqkh34gq3i4gqf"))
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	if v != 7 {
		t.Logf("expected 7, got %d", v)
		t.Fail()
	}
}

// func TestSignature(t *testing.T) {
// 	tm, err := New("monitor-token")
// 	if err != nil {
// 		t.Log(err)
// 		t.Fail()
// 	}

// 	batchID := []byte("0000000ABCEJKLFWEF")
// 	ipb := []byte{108, 61, 56, 35}

// 	sig, err := tm.Sign(1, batchID, ipb)
// 	if err != nil {
// 		t.Log(err)
// 		t.Fail()
// 	}

// 	t.Log(sig)
// }
