// Package privatemode keeps application source on this machine by routing
// source-reading agent phases to a local Muse Glimmer endpoint (Ollama,
// vLLM or llama.cpp, all OpenAI-compatible). It never decides verdicts; it
// only chooses where model traffic may go.
package privatemode

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
)

// EnvMode selects the mode: off (default), local or hybrid.
const EnvMode = "HANDOFF_PRIVATE_MODE"

// Mode is how far model traffic may leave the machine.
type Mode string

const (
	// Off keeps today's routing unchanged.
	Off Mode = "off"
	// Local sends every phase to the local endpoint and refuses any
	// non-loopback request.
	Local Mode = "local"
	// Hybrid keeps the source-reading phases local; phases routed to a
	// cloud endpoint lose every source-reading tool and see evidence only.
	Hybrid Mode = "hybrid"
)

// Local endpoint defaults: Ollama's OpenAI-compatible server.
const (
	DefaultBaseURL = "http://127.0.0.1:11434/v1"
	DefaultModelID = "muse-glimmer"
)

// HybridLocalPhases are the phases that read source and stay local in
// hybrid mode.
var HybridLocalPhases = []string{"intake_summary", "dependency_map", "diagnosis"}

// Parse reads a mode name; empty means Off. Unknown names fail closed.
func Parse(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "", Off:
		return Off, nil
	case Local, Hybrid:
		return m, nil
	default:
		return Off, fmt.Errorf("%s=%q: want off, local or hybrid", EnvMode, s)
	}
}

// FromEnv reads EnvMode.
func FromEnv() (Mode, error) { return Parse(os.Getenv(EnvMode)) }

// LocalRoute resolves the local Glimmer endpoint from the environment:
//
//	HANDOFF_GLIMMER_BASE_URL  default DefaultBaseURL; must be loopback
//	HANDOFF_GLIMMER_MODEL     default DefaultModelID
//	HANDOFF_GLIMMER_API_KEY   optional; local servers ignore it
func LocalRoute() (agent.PhaseRoute, error) {
	base := strings.TrimSuffix(os.Getenv("HANDOFF_GLIMMER_BASE_URL"), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if !agent.IsLoopbackURL(base) {
		return agent.PhaseRoute{}, fmt.Errorf("private mode: HANDOFF_GLIMMER_BASE_URL %q is not a loopback endpoint", base)
	}
	model := os.Getenv("HANDOFF_GLIMMER_MODEL")
	if model == "" {
		model = DefaultModelID
	}
	key := os.Getenv("HANDOFF_GLIMMER_API_KEY")
	if key == "" {
		// Placeholder bearer value: local servers do not check it, and
		// the router fails closed on an empty key.
		key = "local"
	}
	return agent.PhaseRoute{ModelID: model, BaseURL: base, APIKey: key, Provider: agent.ProviderOpenAIChat}, nil
}

// Configure adapts the process config before the router is built. In
// local mode the process default becomes the local route (no Meta API key
// is needed) and the HTTP client refuses every non-loopback host; onBlock
// is told the refused host. Other modes return cfg unchanged.
func Configure(cfg agent.Config, m Mode, local agent.PhaseRoute, onBlock func(host string)) agent.Config {
	if m != Local {
		return cfg
	}
	cfg.ModelID = local.ModelID
	cfg.BaseURL = local.BaseURL
	cfg.APIKey = local.APIKey
	timeout := 120 * time.Second
	var base http.RoundTripper
	if cfg.HTTP != nil {
		timeout = cfg.HTTP.Timeout
		base = cfg.HTTP.Transport
	}
	cfg.HTTP = &http.Client{Timeout: timeout, Transport: Guard(base, onBlock)}
	return cfg
}

// Apply rewrites the router for the mode, then checks that local mode can
// only reach loopback endpoints. It fails closed rather than leak.
func Apply(r *agent.Router, m Mode, local agent.PhaseRoute) error {
	switch m {
	case Off:
		return nil
	case Local:
		r.Default = local
		for _, phase := range agent.Phases {
			r.PerPhase[phase] = local
		}
	case Hybrid:
		for _, phase := range HybridLocalPhases {
			r.PerPhase[phase] = local
		}
	default:
		return fmt.Errorf("private mode: unknown mode %q", m)
	}
	if m == Local {
		for phase, route := range r.PerPhase {
			if !agent.IsLoopbackURL(route.BaseURL) {
				return fmt.Errorf("private mode: phase %s would reach non-loopback %s", phase, route.BaseURL)
			}
		}
	}
	return nil
}

// Wire applies the mode to a runner: routes, the result label, and (in
// hybrid mode) source denial for phases that leave the machine.
func Wire(runner *agent.Runner, r *agent.Router, m Mode, local agent.PhaseRoute) error {
	if err := Apply(r, m, local); err != nil {
		return err
	}
	runner.Router = r
	runner.Mode = string(m)
	runner.SourceLocalOnly = m == Hybrid
	return nil
}

// guard refuses requests to non-loopback hosts before any connection.
type guard struct {
	base    http.RoundTripper
	onBlock func(host string)
}

// Guard wraps base (http.DefaultTransport when nil) so only loopback hosts
// are reachable.
func Guard(base http.RoundTripper, onBlock func(host string)) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &guard{base: base, onBlock: onBlock}
}

func (g *guard) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if !agent.IsLoopbackHost(host) {
		if g.onBlock != nil {
			g.onBlock(host)
		}
		return nil, fmt.Errorf("private mode: egress to %s refused (local mode allows loopback only)", host)
	}
	return g.base.RoundTrip(req)
}
