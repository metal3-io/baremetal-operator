package controllers

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoffIncrements(t *testing.T) {

	var backOff time.Duration
	for i := 1; i <= maxBackOffCount; i++ {
		prev := backOff
		backOff = calculateBackoff(i)

		assert.GreaterOrEqual(t, backOff.Milliseconds(), prev.Milliseconds())
	}

}

func TestMaxBackoffDuration(t *testing.T) {

	maxBackOffDuration := (time.Minute * time.Duration(math.Exp2(float64(maxBackOffCount)))).Milliseconds()

	assert.LessOrEqual(t, calculateBackoff(maxBackOffCount-1).Milliseconds(), maxBackOffDuration)
	assert.LessOrEqual(t, calculateBackoff(maxBackOffCount+1).Milliseconds(), maxBackOffDuration)
	assert.LessOrEqual(t, calculateBackoff(maxBackOffCount+100).Milliseconds(), maxBackOffDuration)
}
