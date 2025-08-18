package retry

import (
	"math"
	"math/rand"
	"time"
)

type Policy interface {
	NextDelay(attempt int) time.Duration
}

type FixedPolicy struct {
	Delay time.Duration
}

func NewFixedPolicy(delay time.Duration) *FixedPolicy {
	return &FixedPolicy{Delay: delay}
}

func (p *FixedPolicy) NextDelay(attempt int) time.Duration {
	return p.Delay
}

type ExponentialPolicy struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Multiplier float64
}

func NewExponentialPolicy(baseDelay, maxDelay time.Duration, multiplier float64) *ExponentialPolicy {
	return &ExponentialPolicy{
		BaseDelay:  baseDelay,
		MaxDelay:   maxDelay,
		Multiplier: multiplier,
	}
}

func (p *ExponentialPolicy) NextDelay(attempt int) time.Duration {
	delay := float64(p.BaseDelay) * math.Pow(p.Multiplier, float64(attempt-1))
	
	if time.Duration(delay) > p.MaxDelay {
		return p.MaxDelay
	}
	
	return time.Duration(delay)
}

type JitterPolicy struct {
	BasePolicy   Policy
	JitterFactor float64
}

func NewJitterPolicy(basePolicy Policy, jitterFactor float64) *JitterPolicy {
	return &JitterPolicy{
		BasePolicy:   basePolicy,
		JitterFactor: jitterFactor,
	}
}

func (p *JitterPolicy) NextDelay(attempt int) time.Duration {
	baseDelay := p.BasePolicy.NextDelay(attempt)
	
	jitter := float64(baseDelay) * p.JitterFactor * (rand.Float64()*2 - 1)
	finalDelay := float64(baseDelay) + jitter
	
	if finalDelay < 0 {
		finalDelay = float64(baseDelay) * 0.1
	}
	
	return time.Duration(finalDelay)
}

func CreatePolicy(policyType string, baseDelay, maxDelay time.Duration, multiplier, jitterFactor float64) Policy {
	var basePolicy Policy
	
	switch policyType {
	case "fixed":
		basePolicy = NewFixedPolicy(baseDelay)
	case "exponential":
		basePolicy = NewExponentialPolicy(baseDelay, maxDelay, multiplier)
	default:
		basePolicy = NewExponentialPolicy(baseDelay, maxDelay, multiplier)
	}
	
	if jitterFactor > 0 {
		return NewJitterPolicy(basePolicy, jitterFactor)
	}
	
	return basePolicy
}