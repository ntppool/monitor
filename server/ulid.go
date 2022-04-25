package server

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"io"
	"log"
	mathrand "math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

type monotonic struct {
	Monotonic *ulid.MonotonicEntropy
}

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
		mono := ulid.Monotonic(rand, inc)
		return mono
	},
}

func makeULID(t time.Time) (*ulid.ULID, error) {

	mono := monotonicPool.Get().(io.Reader)

	id, err := ulid.New(ulid.Timestamp(t), mono)
	if err != nil {
		return nil, err
	}

	return &id, nil
}
