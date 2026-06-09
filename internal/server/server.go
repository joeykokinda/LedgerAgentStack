// Package server exposes the airlock over HTTP: agents POST transaction intents
// and receive either a policy denial or a signed transaction.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/your-handle/agent-airlock/internal/audit"
	"github.com/your-handle/agent-airlock/internal/intent"
	"github.com/your-handle/agent-airlock/internal/policy"
	"github.com/your-handle/agent-airlock/internal/signer"
)

// Server wires the policy engine, signer, and audit logger behind an HTTP API.
type Server struct {
	engine *policy.Engine
	signer signer.Signer
	audit  *audit.Logger
	logf   func(format string, args ...any)
}

// New constructs a Server. If logf is nil, log.Printf is used.
func New(engine *policy.Engine, txSigner signer.Signer, auditLogger *audit.Logger, logf func(format string, args ...any)) *Server {
	if logf == nil {
		logf = log.Printf
	}
	return &Server{
		engine: engine,
		signer: txSigner,
		audit:  auditLogger,
		logf:   logf,
	}
}

// Handler returns the HTTP routes for the airlock.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/transactions", server.handleTransaction)
	mux.HandleFunc("GET /healthz", server.handleHealth)
	return mux
}

func (server *Server) handleHealth(responseWriter http.ResponseWriter, _ *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(responseWriter).Encode(map[string]string{"status": "ok"})
}

// transactionResponse is the success body returned when an intent is signed.
type transactionResponse struct {
	Decision policy.Decision `json:"decision"`
	Signed   signer.SignedTx `json:"signed"`
}

func (server *Server) handleTransaction(responseWriter http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	var txIntent intent.TxIntent
	decoder := json.NewDecoder(http.MaxBytesReader(responseWriter, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&txIntent); err != nil {
		writeError(responseWriter, http.StatusBadRequest, "invalid transaction intent: "+err.Error())
		return
	}
	if err := txIntent.Validate(); err != nil {
		writeError(responseWriter, http.StatusBadRequest, err.Error())
		return
	}

	decision := server.engine.Evaluate(txIntent)

	if !decision.Allowed {
		server.writeAudit(txIntent, decision, nil)
		server.logf("DENY  source=%q chain=%d to=%s rule=%s reason=%q",
			txIntent.Source, txIntent.ChainID, txIntent.To, decision.Rule, decision.Reason)
		writeJSON(responseWriter, http.StatusForbidden, decision)
		return
	}

	signed, err := server.signer.Sign(request.Context(), txIntent)
	if err != nil {
		server.writeAudit(txIntent, decision, audit.ResultFromError(err))
		server.logf("SIGN-FAIL source=%q to=%s error=%q", txIntent.Source, txIntent.To, err.Error())
		writeError(responseWriter, statusForSignerError(err), "signer error: "+err.Error())
		return
	}

	server.writeAudit(txIntent, decision, audit.ResultFromSigned(signed))
	server.logf("ALLOW source=%q chain=%d to=%s signer=%s approved=%t",
		txIntent.Source, txIntent.ChainID, txIntent.To, signed.Signer, signed.DeviceApproved)
	writeJSON(responseWriter, http.StatusOK, transactionResponse{Decision: decision, Signed: signed})
}

func (server *Server) writeAudit(txIntent intent.TxIntent, decision policy.Decision, signerResult *audit.SignerResult) {
	if server.audit == nil {
		return
	}
	if err := server.audit.Log(txIntent, decision, signerResult); err != nil {
		server.logf("audit write failed: %v", err)
	}
}

// statusForSignerError maps a signer failure to an HTTP status. A cancelled or
// deadline-exceeded context surfaces as 504; everything else (device rejection,
// device error) is a 502 bad-gateway from the downstream signer.
func statusForSignerError(err error) int {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func writeJSON(responseWriter http.ResponseWriter, status int, body any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(status)
	_ = json.NewEncoder(responseWriter).Encode(body)
}

func writeError(responseWriter http.ResponseWriter, status int, message string) {
	writeJSON(responseWriter, status, map[string]string{"error": message})
}
