// Package audit writes one structured JSON record per airlock decision so that
// every intent, its verdict, and any signer result form a tamper-evident trail.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/your-handle/agent-airlock/internal/intent"
	"github.com/your-handle/agent-airlock/internal/policy"
	"github.com/your-handle/agent-airlock/internal/signer"
)

// IntentSummary is the subset of a TxIntent worth persisting in the audit log.
type IntentSummary struct {
	ChainID  uint64 `json:"chainId"`
	To       string `json:"to"`
	ValueWei string `json:"valueWei"`
	HasData  bool   `json:"hasData"`
	Nonce    uint64 `json:"nonce"`
}

// SignerResult captures what the signer returned when an intent was forwarded.
type SignerResult struct {
	Forwarded      bool   `json:"forwarded"`
	DeviceApproved bool   `json:"deviceApproved"`
	Signer         string `json:"signer,omitempty"`
	SignedRawTx    string `json:"signedTx,omitempty"`
	Error          string `json:"error,omitempty"`
}

// Record is a single audit entry: the timestamp, intent summary, policy
// decision, and optional signer outcome.
type Record struct {
	Timestamp time.Time     `json:"timestamp"`
	Source    string        `json:"source"`
	Intent    IntentSummary `json:"intent"`
	Decision  string        `json:"decision"`
	Rule      string        `json:"rule"`
	Reason    string        `json:"reason"`
	Signer    *SignerResult `json:"signer,omitempty"`
}

// Logger serialises audit records as newline-delimited JSON to an io.Writer. It
// is safe for concurrent use.
type Logger struct {
	mutex   sync.Mutex
	encoder *json.Encoder
	clock   func() time.Time
}

// NewLogger writes records to sink. Pass nil for clock to use time.Now.
func NewLogger(sink io.Writer, clock func() time.Time) *Logger {
	if clock == nil {
		clock = time.Now
	}
	return &Logger{
		encoder: json.NewEncoder(sink),
		clock:   clock,
	}
}

// Log writes a record describing the decision for the given intent. signerResult
// may be nil when the intent was denied and never forwarded.
func (logger *Logger) Log(txIntent intent.TxIntent, decision policy.Decision, signerResult *SignerResult) error {
	verdict := "deny"
	if decision.Allowed {
		verdict = "allow"
	}
	record := Record{
		Timestamp: logger.clock().UTC(),
		Source:    txIntent.Source,
		Intent:    summarize(txIntent),
		Decision:  verdict,
		Rule:      decision.Rule,
		Reason:    decision.Reason,
		Signer:    signerResult,
	}

	logger.mutex.Lock()
	defer logger.mutex.Unlock()
	if err := logger.encoder.Encode(record); err != nil {
		return fmt.Errorf("write audit record: %w", err)
	}
	return nil
}

// ResultFromSigned builds a SignerResult for a successful signing.
func ResultFromSigned(signed signer.SignedTx) *SignerResult {
	return &SignerResult{
		Forwarded:      true,
		DeviceApproved: signed.DeviceApproved,
		Signer:         signed.Signer,
		SignedRawTx:    signed.SignedRawTx,
	}
}

// ResultFromError builds a SignerResult for a failed signing attempt.
func ResultFromError(err error) *SignerResult {
	return &SignerResult{
		Forwarded:      true,
		DeviceApproved: false,
		Error:          err.Error(),
	}
}

func summarize(txIntent intent.TxIntent) IntentSummary {
	value := "0"
	if txIntent.ValueWei != nil {
		value = txIntent.ValueWei.String()
	}
	return IntentSummary{
		ChainID:  txIntent.ChainID,
		To:       txIntent.To,
		ValueWei: value,
		HasData:  txIntent.HasData(),
		Nonce:    txIntent.Nonce,
	}
}
