package npm

import (
	"context"
	"errors"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/ingestion/native"
)

// Strategy picks which parser implementation to use — the in-process
// Go one that ships in this very package, or the Rust sidecar reached
// over a Unix Domain Socket (see native-ipc.md).
type Strategy string

const (
	StrategyGo     Strategy = "go"
	StrategyNative Strategy = "native"
)

// ErrNativeUnconfigured means the caller asked for StrategyNative but
// didn't wire a native client — treat this as a config error, not a
// runtime fallback.
var ErrNativeUnconfigured = errors.New("npm parser: strategy=native requested but native client is nil")

// StrategyParser is a single facade over both parser backends. Construct
// via NewStrategyParser; dispatch via Parse, which preserves the exact
// semantics of the chosen strategy (no silent fallback — callers that
// want fallback wire it explicitly).
type StrategyParser struct {
	strategy Strategy
	goParser *Parser
	native   *native.Client
}

// NativeClient is the subset of *native.Client we need — declared so
// tests can swap in a fake without spinning up a real UDS server.
type NativeClient interface {
	ParseNPMLockfile(ctx context.Context, content []byte, meta native.LockfileMeta) (*domain.SBOM, error)
	Ping(ctx context.Context) error
}

// NewStrategyParser wires a StrategyParser. Pass nativeClient=nil when
// only the Go strategy is needed; Parse will reject StrategyNative with
// ErrNativeUnconfigured in that case.
func NewStrategyParser(s Strategy, goParser *Parser, nativeClient *native.Client) *StrategyParser {
	return &StrategyParser{
		strategy: s,
		goParser: goParser,
		native:   nativeClient,
	}
}

// Strategy returns the currently-configured strategy.
func (sp *StrategyParser) Strategy() Strategy {
	return sp.strategy
}

// Parse hands the content to the chosen parser backend. Metadata flows
// through unchanged — both parsers stamp ComponentRef / CommitSHA /
// GeneratedAt identically so parity_test.go can diff the outputs.
func (sp *StrategyParser) Parse(ctx context.Context, content []byte, meta LockfileMeta) (*domain.SBOM, error) {
	switch sp.strategy {
	case StrategyNative:
		if sp.native == nil {
			return nil, ErrNativeUnconfigured
		}
		return sp.native.ParseNPMLockfile(ctx, content, native.LockfileMeta{
			ComponentRef: meta.ComponentRef,
			CommitSHA:    meta.CommitSHA,
			GeneratedAt:  meta.GeneratedAt,
		})
	case StrategyGo, "":
		return sp.goParser.Parse(ctx, content, meta)
	default:
		return nil, errors.New("npm parser: unknown strategy " + string(sp.strategy))
	}
}
