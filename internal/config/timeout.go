package config

import (
	"os"
	"strconv"
	"time"
)

// IMDSTimeout returns the timeout to use for IMDS calls. It reads the
// FACTS_IMDS_TIMEOUT environment variable which must be an integer number of
// seconds (e.g. "3"). If not present or invalid, it returns the provided
// fallback.
func IMDSTimeout(fallback time.Duration) time.Duration {
	// Check in-memory override first
	if imdsOverrideSeconds != 0 {
		return time.Duration(imdsOverrideSeconds) * time.Second
	}
	if s := os.Getenv("FACTS_IMDS_TIMEOUT"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if s := os.Getenv("FAX_IMDS_TIMEOUT"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return fallback
}

// EC2Timeout returns the timeout to use for EC2 API calls. It reads the
// FACTS_EC2_TIMEOUT environment variable which must be an integer number of
// seconds (e.g. "10"). If not present or invalid, it returns the provided
// fallback.
func EC2Timeout(fallback time.Duration) time.Duration {
	if ec2OverrideSeconds != 0 {
		return time.Duration(ec2OverrideSeconds) * time.Second
	}
	if s := os.Getenv("FACTS_EC2_TIMEOUT"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if s := os.Getenv("FAX_EC2_TIMEOUT"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return fallback
}

var (
	imdsOverrideSeconds int
	ec2OverrideSeconds  int
)

// SetIMDSTimeoutSeconds sets an in-memory override for the IMDS timeout (seconds).
// Pass 0 to clear the override.
func SetIMDSTimeoutSeconds(sec int) {
	imdsOverrideSeconds = sec
}

// SetEC2TimeoutSeconds sets an in-memory override for the EC2 timeout (seconds).
// Pass 0 to clear the override.
func SetEC2TimeoutSeconds(sec int) {
	ec2OverrideSeconds = sec
}
