package server

import (
	"log"
	"testing"
	"time"
)

func TestULID(t *testing.T) {
	tm := time.Now()
	ul1, err := makeULID(tm)
	if err != nil {
		t.Logf("makeULID failed: %s", err)
		t.Fail()
	}
	ul2, err := makeULID(tm)
	if err != nil {
		t.Logf("makeULID failed: %s", err)
		t.Fail()
	}
	if ul1.String() == ul2.String() {
		t.Logf("ul1 and ul2 got the same string: %s", ul1.String())
		t.Fail()
	}
	log.Printf("ulid string 1 and 2: %s | %s", ul1.String(), ul2.String())
}
