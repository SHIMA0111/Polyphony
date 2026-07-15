package mocks

import (
	"context"
	"sync"
)

// BillingGuardCall records the arguments of a single RecordUsage invocation
// on BillingGuard, for tests asserting exactly what a usecase passed through.
type BillingGuardCall struct {
	RoomID       string
	AIMessageID  string
	Model        string
	PromptTokens int
	OutputTokens int
}

// BillingGuard is a configurable fake implementing message.BillingGuard
// (server/internal/usecase/message) structurally — it deliberately does not
// import that package, mirroring how the interface itself exists so
// usecase/message never has to import usecase/billing.
//
// By default, CheckBalance and RecordUsage both succeed (nil error) and
// RecordUsage calls are appended to RecordUsageCalls for later assertions.
// Set CheckBalanceErr to make CheckBalance return an error (e.g.
// domain.ErrInsufficientBalance), and RecordUsageErr to make RecordUsage
// return an error while still recording the call.
//
// The zero value (mocks.BillingGuard{}) is ready to use.
type BillingGuard struct {
	// CheckBalanceErr, if non-nil, is returned by every CheckBalance call.
	CheckBalanceErr error
	// RecordUsageErr, if non-nil, is returned by every RecordUsage call
	// (after still recording it in RecordUsageCalls).
	RecordUsageErr error

	mu                sync.Mutex
	CheckBalanceCalls []string // roomIDs passed to CheckBalance, in call order
	RecordUsageCalls  []BillingGuardCall
}

// CheckBalance records the call and returns CheckBalanceErr (nil by default).
func (g *BillingGuard) CheckBalance(_ context.Context, roomID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.CheckBalanceCalls = append(g.CheckBalanceCalls, roomID)
	return g.CheckBalanceErr
}

// RecordUsage records the call's arguments and returns RecordUsageErr (nil by default).
func (g *BillingGuard) RecordUsage(_ context.Context, roomID, aiMessageID, model string, promptTokens, outputTokens int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.RecordUsageCalls = append(g.RecordUsageCalls, BillingGuardCall{
		RoomID:       roomID,
		AIMessageID:  aiMessageID,
		Model:        model,
		PromptTokens: promptTokens,
		OutputTokens: outputTokens,
	})
	return g.RecordUsageErr
}

// LastRecordUsageCall returns the most recent RecordUsage call and true, or
// a zero value and false if RecordUsage was never called.
func (g *BillingGuard) LastRecordUsageCall() (BillingGuardCall, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.RecordUsageCalls) == 0 {
		return BillingGuardCall{}, false
	}
	return g.RecordUsageCalls[len(g.RecordUsageCalls)-1], true
}
