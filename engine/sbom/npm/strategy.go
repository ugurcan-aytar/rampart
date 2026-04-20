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
	ParseNPMLockfile(ctx context.Context, content []byte) (*domain.ParsedSBOM, error)
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

// Parse hands the content to the chosen parser backend. Returns a
// ParsedSBOM — both backends produce byte-identical output, enforced
// by parity_test.go.
func (sp *StrategyParser) Parse(ctx context.Context, content []byte) (*domain.ParsedSBOM, error) {
	switch sp.strategy {
	case StrategyNative:
		if sp.native == nil {
			return nil, ErrNativeUnconfigured
		}
		return sp.native.ParseNPMLockfile(ctx, content)
	case StrategyGo, "":
		return sp.goParser.Parse(ctx, content)
	default:
		return nil, errors.New("npm parser: unknown strategy " + string(sp.strategy))
	}
}
