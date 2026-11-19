package chat

import (
	"context"
	"net/http"
	"testing"
	"time"

	"9router/proxy/internal/connlimit"
	"9router/proxy/internal/proxy"
)

func TestAdmitAntigravity_EmptyConnectionID(t *testing.T) {
	rel, err := admitAntigravity(context.Background(), "")
	if err != nil {
		t.Fatalf("empty connection ID failed: %v", err)
	}
	if rel == nil {
		t.Fatal("expected non-nil release func")
	}
	rel() // safe no-op
}

func TestAdmitAntigravity_SequentialTurnPacing(t *testing.T) {
	connID := "conn-pacing-test"
	defer closeAntigravityState(connID)

	// First turn on idle connection proceeds immediately
	t0 := time.Now()
	rel1, err := admitAntigravity(context.Background(), connID)
	if err != nil {
		t.Fatalf("first turn admit failed: %v", err)
	}
	if diff := time.Since(t0); diff > 50*time.Millisecond {
		t.Errorf("first turn took %v, want immediate (<50ms)", diff)
	}

	// Turn 1 completes
	recordAntigravityOutcome(connID, http.StatusOK, nil)
	rel1()

	// Turn 2 arrives 5ms later (rapid bot tool loop)
	t1 := time.Now()
	rel2, err := admitAntigravity(context.Background(), connID)
	if err != nil {
		t.Fatalf("second turn admit failed: %v", err)
	}
	defer rel2()

	elapsed := time.Since(t1)
	// DefaultMinTurnGap is 80ms, so elapsed must be >= 70ms
	if elapsed < 70*time.Millisecond {
		t.Errorf("second turn was not paced: elapsed %v, want >= 70ms", elapsed)
	}
}

func TestAdmitAntigravity_ContextCancellation(t *testing.T) {
	connID := "conn-pacing-cancel"
	defer closeAntigravityState(connID)

	// Simulate turn 1 finishing
	connlimit.RecordTurnFinished(connID)

	// Turn 2 arrives with context timeout shorter than the pacing gap
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := admitAntigravity(ctx, connID)
	if err == nil {
		t.Fatal("expected context deadline exceeded error, got nil")
	}
	if ctx.Err() == nil {
		t.Fatal("expected ctx.Err() to be non-nil")
	}
}

func TestAdmitAntigravity_CircuitBreaker(t *testing.T) {
	connID := "conn-breaker-test"
	defer closeAntigravityState(connID)

	// Trigger 3 consecutive 403 failures to open breaker
	for i := 0; i < 3; i++ {
		recordAntigravityOutcome(connID, http.StatusForbidden, []byte(`{"error":{"code":403,"message":"rate limit"}}`))
	}

	_, err := admitAntigravity(context.Background(), connID)
	if err == nil {
		t.Fatal("expected circuit breaker error, got nil")
	}
	uErr, ok := err.(*proxy.UpstreamError)
	if !ok {
		t.Fatalf("expected *proxy.UpstreamError, got %T: %v", err, err)
	}
	if uErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", uErr.StatusCode)
	}
}
