//go:build integration

package main

import "testing"

func TestEndToEndSample(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run integration sample: %v", err)
	}
}
