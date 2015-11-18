package mesh

import (
	"time"
)

type TokenBucket struct {
	capacity             int64         // Maximum capacity of bucket
	tokenInterval        time.Duration // Token replenishment rate
	refillDuration       time.Duration // Time to refill from empty
	earliestUnspentToken time.Time
}

func NewTokenBucket(capacity int64, tokenInterval time.Duration) *TokenBucket {
	tb := TokenBucket{
		capacity:       capacity,
		tokenInterval:  tokenInterval,
		refillDuration: tokenInterval * time.Duration(capacity)}

	tb.earliestUnspentToken = tb.capacityToken()

	return &tb
}

func (tb *TokenBucket) Wait() {
	// If earliest unspent token is in the future, sleep until then
	time.Sleep(tb.earliestUnspentToken.Sub(time.Now()))

	// Alternatively, enforce bucket capacity if necessary
	capacityToken := tb.capacityToken()
	if tb.earliestUnspentToken.Before(capacityToken) {
		tb.earliestUnspentToken = capacityToken
	}

	// 'Remove' a token from the bucket
	tb.earliestUnspentToken = tb.earliestUnspentToken.Add(tb.tokenInterval)
}

// Determine the historic token timestamp representing a full bucket
func (tb *TokenBucket) capacityToken() time.Time {
	return time.Now().Add(-tb.refillDuration).Truncate(tb.tokenInterval)
}
