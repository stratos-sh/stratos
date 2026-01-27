/*
Copyright 2026 Stratos Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aws

import (
	"context"
	"sync"
	"time"

	"github.com/stratos-sh/stratos/internal/cloudprovider"
)

// RateLimiter implements client-side rate limiting for AWS API calls.
// Based on AWS EC2 API throttling documentation.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// Default rate limits per operation (based on AWS documentation)
var defaultLimits = map[string]struct {
	maxTokens  float64
	refillRate float64
}{
	"RunInstances":          {maxTokens: 100, refillRate: 2},
	"StartInstances":        {maxTokens: 100, refillRate: 2},
	"StopInstances":         {maxTokens: 100, refillRate: 20},
	"TerminateInstances":    {maxTokens: 100, refillRate: 20},
	"DescribeInstances":     {maxTokens: 100, refillRate: 10},
	"DescribeImages":        {maxTokens: 100, refillRate: 10},
	"DescribeSubnets":       {maxTokens: 100, refillRate: 10},
	"DescribeSecurityGroups": {maxTokens: 100, refillRate: 10},
	"IAMInstanceProfile":    {maxTokens: 100, refillRate: 5},
	"CreateTags":            {maxTokens: 100, refillRate: 10},
	"default":               {maxTokens: 100, refillRate: 5},
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*tokenBucket),
	}
}

// Wait waits until a token is available for the specified operation.
func (r *RateLimiter) Wait(ctx context.Context, operation string) error {
	r.mu.Lock()
	bucket := r.getBucket(operation)
	r.mu.Unlock()

	for {
		r.mu.Lock()
		bucket.refill()
		if bucket.tokens >= 1 {
			bucket.tokens--
			r.mu.Unlock()
			return nil
		}
		r.mu.Unlock()

		// Wait and check context
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Check again
		}
	}
}

// TryAcquire attempts to acquire a token without waiting.
func (r *RateLimiter) TryAcquire(operation string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	bucket := r.getBucket(operation)
	bucket.refill()

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}
	return false
}

// getBucket returns the bucket for an operation, creating it if needed.
func (r *RateLimiter) getBucket(operation string) *tokenBucket {
	bucket, ok := r.buckets[operation]
	if ok {
		return bucket
	}

	limits, ok := defaultLimits[operation]
	if !ok {
		limits = defaultLimits["default"]
	}

	bucket = &tokenBucket{
		tokens:     limits.maxTokens,
		maxTokens:  limits.maxTokens,
		refillRate: limits.refillRate,
		lastRefill: time.Now(),
	}
	r.buckets[operation] = bucket
	return bucket
}

// refill adds tokens based on elapsed time.
func (b *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now
}

// ExponentialBackoff performs an operation with exponential backoff.
func ExponentialBackoff(ctx context.Context, maxRetries int, fn func() error) error {
	var lastErr error
	backoff := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if it's a rate limit error
		if _, ok := err.(*cloudprovider.RateLimitError); !ok {
			// Not a rate limit error, don't retry
			return err
		}

		// Wait with exponential backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}

	return lastErr
}
