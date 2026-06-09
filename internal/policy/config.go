package policy

import (
	"fmt"
	"math/big"
	"os"

	"github.com/your-handle/agent-airlock/internal/intent"
	"gopkg.in/yaml.v3"
)

// HourWindow is an inclusive range of UTC hours [Start, End], each 0-23, during
// which transactions are permitted. Start may exceed End to express a window
// that wraps past midnight (e.g. 22-6).
type HourWindow struct {
	Start int `yaml:"start"`
	End   int `yaml:"end"`
}

// Config is the declarative policy loaded from YAML. Wei-denominated fields are
// decimal strings on disk and parsed into big.Int via Compile.
type Config struct {
	MaxValueWei               string     `yaml:"max_value_wei"`
	DailyCapWei               string     `yaml:"daily_cap_wei"`
	AllowedRecipients         []string   `yaml:"allowed_recipients"`
	AllowedChainIDs           []uint64   `yaml:"allowed_chain_ids"`
	AllowedContracts          []string   `yaml:"allowed_contracts"`
	RatePerMinute             int        `yaml:"rate_per_minute"`
	AllowedHoursUTC           HourWindow `yaml:"allowed_hours_utc"`
	RequireRecipientAllowlist bool       `yaml:"require_recipient_allowlist"`
}

// CompiledConfig is the validated, ready-to-evaluate form of a Config. Wei
// limits are big.Int and address lists are normalised sets for O(1) lookup.
type CompiledConfig struct {
	MaxValueWei               *big.Int
	DailyCapWei               *big.Int
	AllowedRecipients         map[string]struct{}
	AllowedChainIDs           map[uint64]struct{}
	AllowedContracts          map[string]struct{}
	RatePerMinute             int
	AllowedHoursUTC           HourWindow
	RequireRecipientAllowlist bool
}

// LoadConfig reads and compiles a policy file at path.
func LoadConfig(path string) (*CompiledConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy %q: %w", path, err)
	}
	var config Config
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("parse policy %q: %w", path, err)
	}
	compiled, err := config.Compile()
	if err != nil {
		return nil, fmt.Errorf("compile policy %q: %w", path, err)
	}
	return compiled, nil
}

// Compile validates the raw config and converts it into a CompiledConfig.
func (config Config) Compile() (*CompiledConfig, error) {
	maxValue, err := intent.ParseValueWei(config.MaxValueWei)
	if err != nil {
		return nil, fmt.Errorf("max_value_wei: %w", err)
	}
	dailyCap, err := intent.ParseValueWei(config.DailyCapWei)
	if err != nil {
		return nil, fmt.Errorf("daily_cap_wei: %w", err)
	}

	recipients, err := normalizeAddressSet(config.AllowedRecipients)
	if err != nil {
		return nil, fmt.Errorf("allowed_recipients: %w", err)
	}
	contracts, err := normalizeAddressSet(config.AllowedContracts)
	if err != nil {
		return nil, fmt.Errorf("allowed_contracts: %w", err)
	}

	if config.RatePerMinute < 0 {
		return nil, fmt.Errorf("rate_per_minute must be non-negative, got %d", config.RatePerMinute)
	}
	if err := validateHourWindow(config.AllowedHoursUTC); err != nil {
		return nil, err
	}

	chainIDs := make(map[uint64]struct{}, len(config.AllowedChainIDs))
	for _, chainID := range config.AllowedChainIDs {
		chainIDs[chainID] = struct{}{}
	}

	return &CompiledConfig{
		MaxValueWei:               maxValue,
		DailyCapWei:               dailyCap,
		AllowedRecipients:         recipients,
		AllowedChainIDs:           chainIDs,
		AllowedContracts:          contracts,
		RatePerMinute:             config.RatePerMinute,
		AllowedHoursUTC:           config.AllowedHoursUTC,
		RequireRecipientAllowlist: config.RequireRecipientAllowlist,
	}, nil
}

func normalizeAddressSet(addresses []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		normalized, err := intent.NormalizeAddress(address)
		if err != nil {
			return nil, err
		}
		set[normalized] = struct{}{}
	}
	return set, nil
}

func validateHourWindow(window HourWindow) error {
	if window.Start < 0 || window.Start > 23 {
		return fmt.Errorf("allowed_hours_utc.start must be 0-23, got %d", window.Start)
	}
	if window.End < 0 || window.End > 23 {
		return fmt.Errorf("allowed_hours_utc.end must be 0-23, got %d", window.End)
	}
	return nil
}

// contains reports whether hour falls within the window, accounting for windows
// that wrap past midnight (Start > End).
func (window HourWindow) contains(hour int) bool {
	if window.Start <= window.End {
		return hour >= window.Start && hour <= window.End
	}
	return hour >= window.Start || hour <= window.End
}
