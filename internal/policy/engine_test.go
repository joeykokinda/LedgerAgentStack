package policy

import (
	"math/big"
	"testing"
	"time"

	"github.com/your-handle/agent-airlock/internal/intent"
)

// allowlistedRecipient and allowlistedContract are referenced across the tests.
const (
	allowlistedRecipient = "0x1111111111111111111111111111111111111111"
	otherRecipient       = "0x2222222222222222222222222222222222222222"
	unlistedRecipient    = "0x9999999999999999999999999999999999999999"
	allowlistedContract  = "0x3333333333333333333333333333333333333333"
)

// baseConfig returns a permissive policy that individual tests tighten as
// needed. Defaults: 1 ETH per tx, 10 ETH/day, recipient allowlist required,
// chain 1 only, 24h window, 5/min.
func baseConfig() Config {
	return Config{
		MaxValueWei:               "1000000000000000000",  // 1 ETH
		DailyCapWei:               "10000000000000000000", // 10 ETH
		AllowedRecipients:         []string{allowlistedRecipient, otherRecipient},
		AllowedChainIDs:           []uint64{1},
		AllowedContracts:          []string{allowlistedContract},
		RatePerMinute:             5,
		AllowedHoursUTC:           HourWindow{Start: 0, End: 23},
		RequireRecipientAllowlist: true,
	}
}

func mustCompile(t *testing.T, config Config) *CompiledConfig {
	t.Helper()
	compiled, err := config.Compile()
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}
	return compiled
}

func wei(t *testing.T, decimal string) *big.Int {
	t.Helper()
	value, err := intent.ParseValueWei(decimal)
	if err != nil {
		t.Fatalf("parse wei %q: %v", decimal, err)
	}
	return value
}

// fixedClock returns a clock stuck at a single instant.
func fixedClock(instant time.Time) Clock {
	return func() time.Time { return instant }
}

// fixedTime is a convenient noon-UTC instant inside the default hour window.
var fixedTime = time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

func baseIntent() intent.TxIntent {
	return intent.TxIntent{
		ChainID:   1,
		To:        allowlistedRecipient,
		ValueWei:  big.NewInt(1000),
		GasLimit:  21000,
		Nonce:     0,
		Source:    "test-agent",
		CreatedAt: fixedTime,
	}
}

func TestEvaluateSingleRules(t *testing.T) {
	testCases := []struct {
		name        string
		mutateCfg   func(*Config)
		mutateTx    func(*intent.TxIntent)
		wantAllowed bool
		wantRule    string
	}{
		{
			name:        "happy path is allowed",
			wantAllowed: true,
			wantRule:    RuleAllowed,
		},
		{
			name:        "chain not allowlisted is denied",
			mutateTx:    func(tx *intent.TxIntent) { tx.ChainID = 999 },
			wantAllowed: false,
			wantRule:    RuleChainAllowlist,
		},
		{
			name:        "chain explicitly allowlisted is allowed",
			mutateCfg:   func(cfg *Config) { cfg.AllowedChainIDs = []uint64{1, 11155111} },
			mutateTx:    func(tx *intent.TxIntent) { tx.ChainID = 11155111 },
			wantAllowed: true,
			wantRule:    RuleAllowed,
		},
		{
			name:        "recipient not on allowlist is denied",
			mutateTx:    func(tx *intent.TxIntent) { tx.To = unlistedRecipient },
			wantAllowed: false,
			wantRule:    RuleRecipientAllowlist,
		},
		{
			name:        "unlisted recipient allowed when allowlist not required",
			mutateCfg:   func(cfg *Config) { cfg.RequireRecipientAllowlist = false },
			mutateTx:    func(tx *intent.TxIntent) { tx.To = unlistedRecipient },
			wantAllowed: true,
			wantRule:    RuleAllowed,
		},
		{
			name:        "recipient allowlist is case insensitive",
			mutateTx:    func(tx *intent.TxIntent) { tx.To = "0X1111111111111111111111111111111111111111" },
			wantAllowed: true,
			wantRule:    RuleAllowed,
		},
		{
			name:        "value over per-tx max is denied",
			mutateTx:    func(tx *intent.TxIntent) { tx.ValueWei = wei(t, "2000000000000000000") },
			wantAllowed: false,
			wantRule:    RuleMaxValue,
		},
		{
			name:        "value exactly at per-tx max is allowed",
			mutateTx:    func(tx *intent.TxIntent) { tx.ValueWei = wei(t, "1000000000000000000") },
			wantAllowed: true,
			wantRule:    RuleAllowed,
		},
		{
			name: "contract call to allowlisted contract is allowed",
			mutateCfg: func(cfg *Config) {
				cfg.RequireRecipientAllowlist = false
			},
			mutateTx: func(tx *intent.TxIntent) {
				tx.To = allowlistedContract
				tx.Data = "0xa9059cbb"
			},
			wantAllowed: true,
			wantRule:    RuleAllowed,
		},
		{
			name: "contract call to non-allowlisted contract is denied",
			mutateCfg: func(cfg *Config) {
				cfg.RequireRecipientAllowlist = false
			},
			mutateTx: func(tx *intent.TxIntent) {
				tx.To = unlistedRecipient
				tx.Data = "0xa9059cbb"
			},
			wantAllowed: false,
			wantRule:    RuleContractAllowlist,
		},
		{
			name:        "empty data 0x is treated as a plain transfer",
			mutateTx:    func(tx *intent.TxIntent) { tx.Data = "0x" },
			wantAllowed: true,
			wantRule:    RuleAllowed,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := baseConfig()
			if testCase.mutateCfg != nil {
				testCase.mutateCfg(&config)
			}
			engine := NewEngine(mustCompile(t, config), fixedClock(fixedTime))

			txIntent := baseIntent()
			if testCase.mutateTx != nil {
				testCase.mutateTx(&txIntent)
			}

			decision := engine.Evaluate(txIntent)
			if decision.Allowed != testCase.wantAllowed {
				t.Fatalf("Allowed = %t, want %t (rule=%s reason=%q)",
					decision.Allowed, testCase.wantAllowed, decision.Rule, decision.Reason)
			}
			if decision.Rule != testCase.wantRule {
				t.Fatalf("Rule = %q, want %q (reason=%q)", decision.Rule, testCase.wantRule, decision.Reason)
			}
		})
	}
}

func TestEvaluateDailyCapDeniesOverCap(t *testing.T) {
	config := baseConfig()
	config.DailyCapWei = "500"
	engine := NewEngine(mustCompile(t, config), fixedClock(fixedTime))

	txIntent := baseIntent()
	txIntent.ValueWei = big.NewInt(1000) // under 1 ETH per-tx max, over the 500 daily cap

	decision := engine.Evaluate(txIntent)
	if decision.Allowed {
		t.Fatalf("expected denial over daily cap, got allow")
	}
	if decision.Rule != RuleDailyCap {
		t.Fatalf("Rule = %q, want %q", decision.Rule, RuleDailyCap)
	}
}

func TestEvaluateOutsideAllowedHoursDenied(t *testing.T) {
	config := baseConfig()
	config.AllowedHoursUTC = HourWindow{Start: 9, End: 17}
	engine := NewEngine(mustCompile(t, config), fixedClock(fixedTime))

	// 12:00 is inside the window -> allowed.
	if decision := engine.Evaluate(baseIntent()); !decision.Allowed {
		t.Fatalf("noon should be inside 9-17 window, got deny: %q", decision.Reason)
	}

	// 03:00 is outside the window -> denied.
	nightTime := time.Date(2026, 6, 8, 3, 0, 0, 0, time.UTC)
	engineNight := NewEngine(mustCompile(t, config), fixedClock(nightTime))
	decision := engineNight.Evaluate(baseIntent())
	if decision.Allowed {
		t.Fatalf("03:00 should be outside 9-17 window, got allow")
	}
	if decision.Rule != RuleAllowedHours {
		t.Fatalf("Rule = %q, want %q", decision.Rule, RuleAllowedHours)
	}
}

func TestEvaluateWrappingHourWindow(t *testing.T) {
	config := baseConfig()
	config.AllowedHoursUTC = HourWindow{Start: 22, End: 6} // wraps midnight

	insideTimes := []time.Time{
		time.Date(2026, 6, 8, 23, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 8, 2, 0, 0, 0, time.UTC),
	}
	for _, instant := range insideTimes {
		engine := NewEngine(mustCompile(t, config), fixedClock(instant))
		if decision := engine.Evaluate(baseIntent()); !decision.Allowed {
			t.Fatalf("hour %02d should be inside wrapping window 22-6: %q", instant.Hour(), decision.Reason)
		}
	}

	noonEngine := NewEngine(mustCompile(t, config), fixedClock(fixedTime))
	if decision := noonEngine.Evaluate(baseIntent()); decision.Allowed {
		t.Fatalf("noon should be outside wrapping window 22-6")
	}
}

func TestEvaluateDailyCapAccumulatesAcrossCalls(t *testing.T) {
	config := baseConfig()
	config.DailyCapWei = "2500" // allows 1000 + 1000, denies the third 1000
	engine := NewEngine(mustCompile(t, config), fixedClock(fixedTime))

	for callIndex := 0; callIndex < 2; callIndex++ {
		decision := engine.Evaluate(baseIntent())
		if !decision.Allowed {
			t.Fatalf("call %d should be allowed (under cumulative cap): %q", callIndex, decision.Reason)
		}
	}

	if got := engine.SpentToday(); got.Cmp(big.NewInt(2000)) != 0 {
		t.Fatalf("SpentToday = %s, want 2000", got)
	}

	decision := engine.Evaluate(baseIntent())
	if decision.Allowed {
		t.Fatalf("third call should breach daily cap (2000 + 1000 > 2500)")
	}
	if decision.Rule != RuleDailyCap {
		t.Fatalf("Rule = %q, want %q", decision.Rule, RuleDailyCap)
	}

	// A denied call must not have mutated the running total.
	if got := engine.SpentToday(); got.Cmp(big.NewInt(2000)) != 0 {
		t.Fatalf("SpentToday after denial = %s, want 2000 (denied spend must not accumulate)", got)
	}
}

func TestEvaluateDailyCapResetsNextDay(t *testing.T) {
	config := baseConfig()
	config.DailyCapWei = "1500"

	movableTime := fixedTime
	engine := NewEngine(mustCompile(t, config), func() time.Time { return movableTime })

	if decision := engine.Evaluate(baseIntent()); !decision.Allowed {
		t.Fatalf("first day first call should pass: %q", decision.Reason)
	}
	if decision := engine.Evaluate(baseIntent()); decision.Allowed {
		t.Fatalf("first day second call should breach the 1500 cap")
	}

	// Advance to the next UTC day; the running total must reset.
	movableTime = fixedTime.Add(24 * time.Hour)
	if decision := engine.Evaluate(baseIntent()); !decision.Allowed {
		t.Fatalf("next day call should pass after daily reset: %q", decision.Reason)
	}
	if got := engine.SpentToday(); got.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("SpentToday on next day = %s, want 1000 after reset", got)
	}
}

func TestEvaluateRateLimitTrips(t *testing.T) {
	config := baseConfig()
	config.RatePerMinute = 3
	// Generous cap so the rate limit, not the cap, is what trips.
	config.DailyCapWei = "1000000000000000000000"

	movableTime := fixedTime
	engine := NewEngine(mustCompile(t, config), func() time.Time { return movableTime })

	for callIndex := 0; callIndex < 3; callIndex++ {
		movableTime = fixedTime.Add(time.Duration(callIndex) * time.Second)
		decision := engine.Evaluate(baseIntent())
		if !decision.Allowed {
			t.Fatalf("call %d within rate limit should pass: %q", callIndex, decision.Reason)
		}
	}

	// Fourth call inside the same minute trips the limit.
	movableTime = fixedTime.Add(4 * time.Second)
	decision := engine.Evaluate(baseIntent())
	if decision.Allowed {
		t.Fatalf("fourth call in one minute should trip the rate limit")
	}
	if decision.Rule != RuleRateLimit {
		t.Fatalf("Rule = %q, want %q", decision.Rule, RuleRateLimit)
	}
}

func TestEvaluateRateLimitWindowSlides(t *testing.T) {
	config := baseConfig()
	config.RatePerMinute = 2
	config.DailyCapWei = "1000000000000000000000"

	movableTime := fixedTime
	engine := NewEngine(mustCompile(t, config), func() time.Time { return movableTime })

	// Two approvals exhaust the per-minute budget.
	for callIndex := 0; callIndex < 2; callIndex++ {
		if decision := engine.Evaluate(baseIntent()); !decision.Allowed {
			t.Fatalf("call %d should pass: %q", callIndex, decision.Reason)
		}
	}

	// Immediately blocked.
	if decision := engine.Evaluate(baseIntent()); decision.Allowed {
		t.Fatalf("third immediate call should be rate limited")
	}

	// After the window slides past the earliest approvals, a call is allowed.
	movableTime = fixedTime.Add(61 * time.Second)
	if decision := engine.Evaluate(baseIntent()); !decision.Allowed {
		t.Fatalf("call after the minute window should pass: %q", decision.Reason)
	}
}

func TestEvaluateRuleOrderingShortCircuits(t *testing.T) {
	// An intent that violates several rules at once should report the first one
	// in evaluation order (chain before recipient before value ...).
	config := baseConfig()
	engine := NewEngine(mustCompile(t, config), fixedClock(fixedTime))

	txIntent := baseIntent()
	txIntent.ChainID = 999                            // bad chain (checked first)
	txIntent.To = unlistedRecipient                   // also not allowlisted
	txIntent.ValueWei = wei(t, "5000000000000000000") // also over max

	decision := engine.Evaluate(txIntent)
	if decision.Allowed {
		t.Fatalf("multi-violation intent must be denied")
	}
	if decision.Rule != RuleChainAllowlist {
		t.Fatalf("Rule = %q, want %q (chain check must short-circuit first)", decision.Rule, RuleChainAllowlist)
	}
}

func TestEvaluateZeroRatePerMinuteDisablesLimit(t *testing.T) {
	config := baseConfig()
	config.RatePerMinute = 0 // disabled
	config.DailyCapWei = "1000000000000000000000"
	engine := NewEngine(mustCompile(t, config), fixedClock(fixedTime))

	for callIndex := 0; callIndex < 50; callIndex++ {
		if decision := engine.Evaluate(baseIntent()); !decision.Allowed {
			t.Fatalf("call %d should pass with rate limiting disabled: %q", callIndex, decision.Reason)
		}
	}
}

func TestCompileRejectsBadInput(t *testing.T) {
	testCases := []struct {
		name      string
		mutateCfg func(*Config)
	}{
		{"bad max value", func(cfg *Config) { cfg.MaxValueWei = "not-a-number" }},
		{"bad daily cap", func(cfg *Config) { cfg.DailyCapWei = "-5" }},
		{"bad recipient address", func(cfg *Config) { cfg.AllowedRecipients = []string{"0xnothex"} }},
		{"bad contract address", func(cfg *Config) { cfg.AllowedContracts = []string{"short"} }},
		{"negative rate", func(cfg *Config) { cfg.RatePerMinute = -1 }},
		{"hour out of range", func(cfg *Config) { cfg.AllowedHoursUTC = HourWindow{Start: 0, End: 99} }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := baseConfig()
			testCase.mutateCfg(&config)
			if _, err := config.Compile(); err == nil {
				t.Fatalf("expected compile error for %s, got nil", testCase.name)
			}
		})
	}
}
