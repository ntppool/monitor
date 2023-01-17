package ulid

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"io"
	"log"
	mathrand "math/rand"
	"sync"
	"time"

	oklid "github.com/oklog/ulid/v2"
)

var monotonicPool = sync.Pool{
	New: func() interface{} {

		var seed int64
		err := binary.Read(cryptorand.Reader, binary.BigEndian, &seed)
		if err != nil {
			log.Fatalf("crypto/rand error: %s", err)
		}

		rand := mathrand.New(mathrand.NewSource(seed))

		inc := uint64(mathrand.Int63())

		// log.Printf("seed: %d", seed)
		// log.Printf("inc:  %d", inc)

		// inc = inc & ^uint64(1<<63) // only want 63 bits
		mono := oklid.Monotonic(rand, inc)
		return mono
	},
}

func MakeULID(t time.Time) (*oklid.ULID, error) {

	mono := monotonicPool.Get().(io.Reader)

	id, err := oklid.New(oklid.Timestamp(t), mono)
	if err != nil {
		return nil, err
	}

	return &id, nil
}
