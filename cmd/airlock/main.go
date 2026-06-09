// Command airlock runs the policy firewall HTTP server that sits between an
// autonomous agent and a Ledger hardware signer.
package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/your-handle/agent-airlock/internal/audit"
	"github.com/your-handle/agent-airlock/internal/policy"
	"github.com/your-handle/agent-airlock/internal/server"
	"github.com/your-handle/agent-airlock/internal/signer"
)

func main() {
	policyPath := flag.String("policy", "policy/example-policy.yaml", "path to the YAML policy file")
	listenAddr := flag.String("addr", ":8080", "address the airlock HTTP server listens on")
	signerCmd := flag.String("signer-cmd", "", "external signer command (argv, space-separated); empty uses the mock signer")
	auditLogPath := flag.String("audit-log", "-", `audit log destination ("-" for stdout, or a file path)`)
	flag.Parse()

	logger := log.New(os.Stderr, "airlock ", log.LstdFlags|log.LUTC)

	compiledPolicy, err := policy.LoadConfig(*policyPath)
	if err != nil {
		logger.Fatalf("load policy: %v", err)
	}

	auditSink, closeAudit, err := openAuditSink(*auditLogPath)
	if err != nil {
		logger.Fatalf("open audit log: %v", err)
	}
	defer closeAudit()

	engine := policy.NewEngine(compiledPolicy, nil)
	auditLogger := audit.NewLogger(auditSink, nil)
	txSigner := buildSigner(*signerCmd, logger)

	airlockServer := server.New(engine, txSigner, auditLogger, logger.Printf)

	logStartup(logger, *policyPath, *listenAddr, *signerCmd, *auditLogPath, compiledPolicy)

	httpServer := &http.Server{
		Addr:    *listenAddr,
		Handler: airlockServer.Handler(),
	}
	if err := httpServer.ListenAndServe(); err != nil {
		logger.Fatalf("server stopped: %v", err)
	}
}

func buildSigner(signerCmd string, logger *log.Logger) signer.Signer {
	argv := strings.Fields(signerCmd)
	if len(argv) == 0 {
		mock := signer.NewMockSigner()
		mock.Logf = logger.Printf
		return mock
	}
	commandSigner, err := signer.NewCommandSigner(argv)
	if err != nil {
		logger.Fatalf("build signer: %v", err)
	}
	return commandSigner
}

func openAuditSink(path string) (io.Writer, func(), error) {
	if path == "-" || path == "" {
		return os.Stdout, func() {}, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}

func logStartup(logger *log.Logger, policyPath, listenAddr, signerCmd, auditLogPath string, compiledPolicy *policy.CompiledConfig) {
	signerMode := "mock (no device)"
	if strings.TrimSpace(signerCmd) != "" {
		signerMode = "command: " + signerCmd
	}

	logger.Printf("Agent Airlock starting")
	logger.Printf("  policy file       : %s", policyPath)
	logger.Printf("  listen address    : %s", listenAddr)
	logger.Printf("  signer            : %s", signerMode)
	logger.Printf("  audit log         : %s", auditLogPath)
	logger.Printf("policy limits:")
	logger.Printf("  max value (wei)   : %s", compiledPolicy.MaxValueWei.String())
	logger.Printf("  daily cap (wei)   : %s", compiledPolicy.DailyCapWei.String())
	logger.Printf("  rate per minute   : %d", compiledPolicy.RatePerMinute)
	logger.Printf("  allowed hours UTC : %02d-%02d", compiledPolicy.AllowedHoursUTC.Start, compiledPolicy.AllowedHoursUTC.End)
	logger.Printf("  require allowlist : %t", compiledPolicy.RequireRecipientAllowlist)
	logger.Printf("  allowed chains    : %s", joinUints(compiledPolicy.AllowedChainIDs))
	logger.Printf("  allowed recipients: %d configured", len(compiledPolicy.AllowedRecipients))
	logger.Printf("  allowed contracts : %d configured", len(compiledPolicy.AllowedContracts))
}

func joinUints(values map[uint64]struct{}) string {
	if len(values) == 0 {
		return "(none)"
	}
	sorted := make([]uint64, 0, len(values))
	for value := range values {
		sorted = append(sorted, value)
	}
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })

	parts := make([]string, len(sorted))
	for index, value := range sorted {
		parts[index] = strconv.FormatUint(value, 10)
	}
	return strings.Join(parts, ", ")
}
