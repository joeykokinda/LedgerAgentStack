// Package policy implements the deterministic software gate that evaluates an
// agent's transaction intent against declarative rules before it can reach the
// hardware signer.
package policy

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/your-handle/agent-airlock/internal/intent"
)

// Rule names identify which check produced a decision. They are stable strings
// suitable for audit logs and assertions.
const (
	RuleChainAllowlist     = "chain_allowlist"
	RuleRecipientAllowlist = "recipient_allowlist"
	RuleMaxValue           = "max_value"
	RuleDailyCap           = "daily_cap"
	RuleContractAllowlist  = "contract_allowlist"
	RuleAllowedHours       = "allowed_hours"
	RuleRateLimit          = "rate_limit"
	RuleAllowed            = "allowed"
)

// Decision is the verdict the engine returns for a single intent. Rule names
// the rule that decided the outcome; Reason is a human-readable explanation.
type Decision struct {
	Allowed bool   `json:"allowed"`
	Rule    string `json:"rule"`
	Reason  string `json:"reason"`
}

// Clock returns the current time. Injecting it keeps the daily-cap and
// rate-limit windows deterministic in tests.
type Clock func() time.Time

// Engine evaluates intents against a CompiledConfig while tracking per-day
// spend and a sliding-window request rate. It is safe for concurrent use.
type Engine struct {
	config *CompiledConfig
	clock  Clock

	mutex           sync.Mutex
	spentToday      *big.Int
	currentSpendDay string
	recentApprovals []time.Time
}

// NewEngine builds an engine for the given policy. If clock is nil, time.Now is
// used.
func NewEngine(config *CompiledConfig, clock Clock) *Engine {
	if clock == nil {
		clock = time.Now
	}
	return &Engine{
		config:     config,
		clock:      clock,
		spentToday: big.NewInt(0),
	}
}

// Evaluate applies every policy rule in order and returns the first failure, or
// an allow decision. On allow it records the spend and the approval timestamp
// so that subsequent calls see the updated daily total and rate window.
func (engine *Engine) Evaluate(txIntent intent.TxIntent) Decision {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()

	now := engine.clock().UTC()
	engine.rollDailyWindow(now)

	if _, ok := engine.config.AllowedChainIDs[txIntent.ChainID]; !ok {
		return deny(RuleChainAllowlist, "chain id %d is not in the allowed_chain_ids list", txIntent.ChainID)
	}

	recipient, err := intent.NormalizeAddress(txIntent.To)
	if err != nil {
		return deny(RuleRecipientAllowlist, "recipient address is malformed: %v", err)
	}
	if engine.config.RequireRecipientAllowlist {
		if _, ok := engine.config.AllowedRecipients[recipient]; !ok {
			return deny(RuleRecipientAllowlist, "recipient %s is not on the allowlist", recipient)
		}
	}

	value := txIntent.ValueWei
	if value == nil {
		value = big.NewInt(0)
	}
	if value.Cmp(engine.config.MaxValueWei) > 0 {
		return deny(RuleMaxValue, "value %s wei exceeds max_value_wei %s", value.String(), engine.config.MaxValueWei.String())
	}

	projectedSpend := new(big.Int).Add(engine.spentToday, value)
	if projectedSpend.Cmp(engine.config.DailyCapWei) > 0 {
		return deny(RuleDailyCap,
			"daily spend would reach %s wei, exceeding daily_cap_wei %s (already spent %s today)",
			projectedSpend.String(), engine.config.DailyCapWei.String(), engine.spentToday.String())
	}

	if txIntent.HasData() {
		if _, ok := engine.config.AllowedContracts[recipient]; !ok {
			return deny(RuleContractAllowlist, "contract call to %s is not on the allowed_contracts list", recipient)
		}
	}

	if !engine.config.AllowedHoursUTC.contains(now.Hour()) {
		return deny(RuleAllowedHours,
			"current hour %02d UTC is outside the allowed window %02d-%02d",
			now.Hour(), engine.config.AllowedHoursUTC.Start, engine.config.AllowedHoursUTC.End)
	}

	if engine.config.RatePerMinute > 0 {
		approvalsInWindow := engine.countRecentApprovals(now)
		if approvalsInWindow >= engine.config.RatePerMinute {
			return deny(RuleRateLimit,
				"rate limit reached: %d approvals in the last minute, limit is %d per minute",
				approvalsInWindow, engine.config.RatePerMinute)
		}
	}

	engine.spentToday = projectedSpend
	engine.recentApprovals = append(engine.recentApprovals, now)
	return Decision{Allowed: true, Rule: RuleAllowed, Reason: "all policy checks passed"}
}

// SpentToday returns the running daily total in wei. Intended for diagnostics
// and tests.
func (engine *Engine) SpentToday() *big.Int {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	engine.rollDailyWindow(engine.clock().UTC())
	return new(big.Int).Set(engine.spentToday)
}

// rollDailyWindow resets the daily spend total when the UTC calendar day
// changes. The caller must hold the mutex.
func (engine *Engine) rollDailyWindow(now time.Time) {
	day := now.Format("2006-01-02")
	if day != engine.currentSpendDay {
		engine.currentSpendDay = day
		engine.spentToday = big.NewInt(0)
	}
}

// countRecentApprovals returns how many approvals fall within the trailing
// 60-second window and compacts older entries out of the slice. The caller must
// hold the mutex.
func (engine *Engine) countRecentApprovals(now time.Time) int {
	cutoff := now.Add(-time.Minute)
	kept := engine.recentApprovals[:0]
	for _, approvedAt := range engine.recentApprovals {
		if approvedAt.After(cutoff) {
			kept = append(kept, approvedAt)
		}
	}
	engine.recentApprovals = kept
	return len(kept)
}

func deny(rule, format string, args ...any) Decision {
	return Decision{
		Allowed: false,
		Rule:    rule,
		Reason:  fmt.Sprintf(format, args...),
	}
}
