package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShortDuration(t *testing.T) {
	assert.Equal(t, "24h", shortDuration(24*time.Hour))
	assert.Equal(t, "1h30m", shortDuration(90*time.Minute))
	assert.Equal(t, "45s", shortDuration(45*time.Second))
	assert.Equal(t, "2m30s", shortDuration(150*time.Second))
}
