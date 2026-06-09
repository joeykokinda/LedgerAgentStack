// Package intent defines the transaction intent that an autonomous agent
// emits and that the airlock evaluates before any hardware signing occurs.
package intent

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// hexAddressPattern matches a 20-byte Ethereum-style address in hex form. The
// 0x prefix may be upper- or lower-case; NormalizeAddress lower-cases the
// result so allowlist comparisons are case-insensitive.
var hexAddressPattern = regexp.MustCompile(`^0[xX][0-9a-fA-F]{40}$`)

// TxIntent is the agent's declared intention to send a transaction. It is the
// unit of work that flows through the airlock: agent -> policy -> signer.
type TxIntent struct {
	ChainID   uint64    `json:"chainId"`
	To        string    `json:"to"`
	ValueWei  *big.Int  `json:"-"`
	Data      string    `json:"data,omitempty"`
	GasLimit  uint64    `json:"gasLimit"`
	Nonce     uint64    `json:"nonce"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

// txIntentJSON mirrors TxIntent for the wire, serialising ValueWei as a
// decimal string so arbitrary-precision values survive JSON round-trips.
type txIntentJSON struct {
	ChainID   uint64    `json:"chainId"`
	To        string    `json:"to"`
	ValueWei  string    `json:"valueWei"`
	Data      string    `json:"data,omitempty"`
	GasLimit  uint64    `json:"gasLimit"`
	Nonce     uint64    `json:"nonce"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

// MarshalJSON encodes the intent with ValueWei as a decimal string.
func (intent TxIntent) MarshalJSON() ([]byte, error) {
	value := "0"
	if intent.ValueWei != nil {
		value = intent.ValueWei.String()
	}
	return json.Marshal(txIntentJSON{
		ChainID:   intent.ChainID,
		To:        intent.To,
		ValueWei:  value,
		Data:      intent.Data,
		GasLimit:  intent.GasLimit,
		Nonce:     intent.Nonce,
		Source:    intent.Source,
		CreatedAt: intent.CreatedAt,
	})
}

// UnmarshalJSON decodes the intent, parsing ValueWei from a decimal string.
func (intent *TxIntent) UnmarshalJSON(data []byte) error {
	var wire txIntentJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode tx intent: %w", err)
	}
	value, err := ParseValueWei(wire.ValueWei)
	if err != nil {
		return fmt.Errorf("decode tx intent: %w", err)
	}
	intent.ChainID = wire.ChainID
	intent.To = wire.To
	intent.ValueWei = value
	intent.Data = wire.Data
	intent.GasLimit = wire.GasLimit
	intent.Nonce = wire.Nonce
	intent.Source = wire.Source
	intent.CreatedAt = wire.CreatedAt
	return nil
}

// ParseValueWei parses a decimal wei string into a non-negative big.Int. An
// empty string is treated as zero.
func ParseValueWei(decimal string) (*big.Int, error) {
	trimmed := strings.TrimSpace(decimal)
	if trimmed == "" {
		return big.NewInt(0), nil
	}
	value, ok := new(big.Int).SetString(trimmed, 10)
	if !ok {
		return nil, fmt.Errorf("invalid wei value %q: not a base-10 integer", decimal)
	}
	if value.Sign() < 0 {
		return nil, fmt.Errorf("invalid wei value %q: must be non-negative", decimal)
	}
	return value, nil
}

// NormalizeAddress validates a hex address and returns it lower-cased so that
// allowlist comparisons are case-insensitive.
func NormalizeAddress(address string) (string, error) {
	if !hexAddressPattern.MatchString(address) {
		return "", fmt.Errorf("invalid hex address %q: want 0x-prefixed 20-byte hex", address)
	}
	return strings.ToLower(address), nil
}

// HasData reports whether the intent carries non-empty call data, i.e. it is a
// contract interaction rather than a plain value transfer.
func (intent TxIntent) HasData() bool {
	trimmed := strings.TrimSpace(intent.Data)
	return trimmed != "" && trimmed != "0x" && trimmed != "0X"
}

// Validate performs structural checks independent of any policy: the recipient
// must be a well-formed address and the value must be present.
func (intent TxIntent) Validate() error {
	if _, err := NormalizeAddress(intent.To); err != nil {
		return fmt.Errorf("validate intent: %w", err)
	}
	if intent.ValueWei == nil {
		return fmt.Errorf("validate intent: missing value")
	}
	if intent.ValueWei.Sign() < 0 {
		return fmt.Errorf("validate intent: value must be non-negative")
	}
	return nil
}
