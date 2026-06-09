// Package signer forwards a policy-approved intent to a transaction signer. The
// production signer shells out to an external process (e.g. a Node script
// driving a Ledger device via speculos); the mock signer is used for tests and
// local demos.
package signer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/your-handle/agent-airlock/internal/intent"
)

// SignedTx is the result of forwarding an intent to the signer. DeviceApproved
// reflects whether the human confirmed on the hardware screen.
type SignedTx struct {
	SignedRawTx    string `json:"signedTx"`
	DeviceApproved bool   `json:"deviceApproved"`
	Signer         string `json:"signer"`
}

// Signer turns an approved intent into a signed transaction.
type Signer interface {
	Sign(ctx context.Context, txIntent intent.TxIntent) (SignedTx, error)
}

// signerResponse is the JSON contract the external command writes to stdout.
type signerResponse struct {
	SignedTx       string `json:"signedTx"`
	DeviceApproved bool   `json:"deviceApproved"`
}

// CommandSigner runs an external command, writing the TxIntent as JSON to the
// child's stdin and reading {"signedTx":..,"deviceApproved":..} from stdout. A
// non-zero exit represents the user rejecting on the device or a device error
// and is surfaced as an error wrapping stderr.
type CommandSigner struct {
	// Argv is the command and its arguments, e.g. ["node", "sign.js"].
	Argv []string
	// Name labels the signer in audit records and the returned SignedTx.
	Name string
}

// NewCommandSigner builds a CommandSigner from an argv slice. The slice must be
// non-empty.
func NewCommandSigner(argv []string) (*CommandSigner, error) {
	if len(argv) == 0 {
		return nil, errors.New("command signer requires a non-empty argv")
	}
	return &CommandSigner{Argv: argv, Name: "command:" + argv[0]}, nil
}

// Sign forwards the intent to the external command and decodes its response.
func (commandSigner *CommandSigner) Sign(ctx context.Context, txIntent intent.TxIntent) (SignedTx, error) {
	requestBody, err := json.Marshal(txIntent)
	if err != nil {
		return SignedTx{}, fmt.Errorf("encode intent for signer: %w", err)
	}

	command := exec.CommandContext(ctx, commandSigner.Argv[0], commandSigner.Argv[1:]...)
	command.Stdin = bytes.NewReader(requestBody)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return SignedTx{}, fmt.Errorf("signer command failed (device rejected or error): %s: %w", message, err)
	}

	var response signerResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return SignedTx{}, fmt.Errorf("decode signer response %q: %w", strings.TrimSpace(stdout.String()), err)
	}
	if !response.DeviceApproved {
		return SignedTx{}, fmt.Errorf("signer reported the device did not approve the transaction")
	}

	return SignedTx{
		SignedRawTx:    response.SignedTx,
		DeviceApproved: response.DeviceApproved,
		Signer:         commandSigner.Name,
	}, nil
}

// MockSigner returns a canned signed transaction without touching any device.
// It is used in tests and whenever no external signer command is configured.
type MockSigner struct {
	// SignedRawTx is the value returned as the signed transaction.
	SignedRawTx string
	// Name labels the signer; defaults to "mock".
	Name string
	// Logf, if set, is called once per Sign to surface the mock behaviour.
	Logf func(format string, args ...any)
}

// NewMockSigner builds a MockSigner with a default canned transaction.
func NewMockSigner() *MockSigner {
	return &MockSigner{
		SignedRawTx: "0xMOCK_SIGNED_TX",
		Name:        "mock",
	}
}

// Sign returns the canned transaction and reports it as device-approved.
func (mockSigner *MockSigner) Sign(_ context.Context, txIntent intent.TxIntent) (SignedTx, error) {
	if mockSigner.Logf != nil {
		mockSigner.Logf("[mock] would sign on device: chain=%d to=%s value=%s wei",
			txIntent.ChainID, txIntent.To, weiString(txIntent))
	}
	name := mockSigner.Name
	if name == "" {
		name = "mock"
	}
	return SignedTx{
		SignedRawTx:    mockSigner.SignedRawTx,
		DeviceApproved: true,
		Signer:         name,
	}, nil
}

func weiString(txIntent intent.TxIntent) string {
	if txIntent.ValueWei == nil {
		return "0"
	}
	return txIntent.ValueWei.String()
}
