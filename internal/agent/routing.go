package agent

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// Phases is the canonical six-phase order of the agent run.
var Phases = []string{
	"intake_summary",
	"dependency_map",
	"diagnosis",
	"repair_proposal",
	"assessment",
	"verification",
}

// Provider kinds. meta-responses is the Meta Responses API used today;
// openai-chat is any OpenAI-compatible chat-completions endpoint.
const (
	ProviderMetaResponses = "meta-responses"
	ProviderOpenAIChat    = "openai-chat"
)

// phaseShort maps each phase to the short name used in per-phase env vars,
// e.g. HANDOFF_MODEL_REPAIR for the repair_proposal phase.
var phaseShort = map[string]string{
	"intake_summary":  "INTAKE",
	"dependency_map":  "DEPENDENCY",
	"diagnosis":       "DIAGNOSIS",
	"repair_proposal": "REPAIR",
	"assessment":      "ASSESSMENT",
	"verification":    "VERIFICATION",
}

// PhaseRoute is how one phase reaches its model. APIKey holds the resolved
// secret value: it is sent on the wire only and never logged.
type PhaseRoute struct {
	ModelID  string
	BaseURL  string
	APIKey   string
	Provider string
}

// Router resolves one PhaseRoute per phase. The zero-value default route
// reproduces today's behaviour exactly (the process Config for every phase).
type Router struct {
	Default  PhaseRoute
	PerPhase map[string]PhaseRoute
}

// RouterFromEnv builds per-phase routes from the process Config plus optional
// per-phase overrides:
//
//	HANDOFF_MODEL_<SHORT>    model ID (default: META_MODEL_ID)
//	HANDOFF_PROVIDER_<SHORT> meta-responses | openai-chat (default: meta-responses)
//	HANDOFF_BASE_URL_<SHORT> endpoint base URL (default: META_API_BASE_URL)
//	HANDOFF_KEY_ENV_<SHORT>  name of the env var holding this phase's key
//	                         (default: the key already in def)
//
// Unknown provider kinds and unresolvable keys fail closed.
func RouterFromEnv(def Config) (Router, error) {
	r := Router{
		Default: PhaseRoute{
			ModelID:  def.ModelID,
			BaseURL:  strings.TrimSuffix(def.BaseURL, "/"),
			APIKey:   def.APIKey,
			Provider: ProviderMetaResponses,
		},
		PerPhase: map[string]PhaseRoute{},
	}
	if r.Default.ModelID == "" {
		return r, fmt.Errorf("META_MODEL_ID is not set (see .env.example)")
	}
	if r.Default.BaseURL == "" {
		return r, fmt.Errorf("model base URL is not set (see .env.example)")
	}
	if r.Default.APIKey == "" {
		return r, fmt.Errorf("META_API_KEY is not set (see .env.example)")
	}
	for _, phase := range Phases {
		short, ok := phaseShort[phase]
		if !ok {
			return r, fmt.Errorf("no env short name for phase %q", phase)
		}
		route := r.Default
		if v := os.Getenv("HANDOFF_MODEL_" + short); v != "" {
			route.ModelID = v
		}
		if v := os.Getenv("HANDOFF_BASE_URL_" + short); v != "" {
			route.BaseURL = strings.TrimSuffix(v, "/")
		}
		if v := os.Getenv("HANDOFF_PROVIDER_" + short); v != "" {
			route.Provider = v
		}
		if v := os.Getenv("HANDOFF_KEY_ENV_" + short); v != "" {
			route.APIKey = os.Getenv(v)
		}
		if err := route.validate(phase); err != nil {
			return r, err
		}
		r.PerPhase[phase] = route
	}
	return r, nil
}

// ForPhase resolves the route for one phase, failing closed on unknown
// phases and unknown provider kinds.
func (r Router) ForPhase(phase string) (PhaseRoute, error) {
	route, ok := r.PerPhase[phase]
	if !ok {
		if r.PerPhase == nil {
			route = r.Default
		} else {
			return PhaseRoute{}, fmt.Errorf("unknown phase %q", phase)
		}
	}
	if err := route.validate(phase); err != nil {
		return PhaseRoute{}, err
	}
	return route, nil
}

func (p PhaseRoute) validate(phase string) error {
	if p.ModelID == "" {
		return fmt.Errorf("phase %s: model ID is not set", phase)
	}
	if p.BaseURL == "" {
		return fmt.Errorf("phase %s: base URL is not set", phase)
	}
	if p.APIKey == "" {
		return fmt.Errorf("phase %s: API key is not set", phase)
	}
	switch p.Provider {
	case ProviderMetaResponses, ProviderOpenAIChat:
		return nil
	default:
		return fmt.Errorf("phase %s: unknown provider kind %q (want %q or %q)",
			phase, p.Provider, ProviderMetaResponses, ProviderOpenAIChat)
	}
}

// IsLoopbackURL reports whether rawURL targets this machine (localhost or a
// loopback IP). Private mode uses it to keep source-reading traffic local.
func IsLoopbackURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return IsLoopbackHost(u.Hostname())
}

// IsLoopbackHost reports whether host is localhost or a loopback IP.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
