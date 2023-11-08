package util

import (
	"strings"
	"time"
)

func FormatDuration(duration time.Duration) string {
	return FormatDurationSec(duration.Nanoseconds())
}

func FormatDurationSec(lengthNs int64) string {
	duration := time.Duration(int64(float64(lengthNs)/float64(time.Second.Nanoseconds())+0.5) * time.Second.Nanoseconds())
	stringDuration := duration.String()
	if strings.HasSuffix(stringDuration, "m0s") {
		stringDuration = stringDuration[:len(stringDuration)-2]
	}
	if strings.HasSuffix(stringDuration, "h0m") {
		stringDuration = stringDuration[:len(stringDuration)-2]
	}
	return stringDuration
}
