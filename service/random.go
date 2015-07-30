package service

import (
	crypto "crypto/rand"
	"math/big"
	pseudo "math/rand"
	"time"
)

func init() {
	seed, err := crypto.Int(crypto.Reader, big.NewInt((1<<63)-1))
	if err != nil {
		panic(err)
	}

	pseudo.Seed(seed.Int64())
}

func randomDuration(min, max time.Duration) time.Duration {
	return min + time.Duration(float64(max-min)*pseudo.Float64())
}
