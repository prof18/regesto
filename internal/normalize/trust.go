package normalize

import (
	"sort"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
)

// TrustPolicy resolves normalisation authority from configured integration
// surfaces. It deliberately keys defaults by integration ID, not profile ID:
// two integrations using the same profile can represent different surfaces and
// therefore have different source namespaces and trust defaults.
type TrustPolicy struct {
	integrationTrust map[string]string
	trustedSources   map[string]bool
	exactPolicies    map[string]string
	patterns         []config.SourcePolicyRule
}

// ResolveTrustPolicy builds one policy for a normalisation run. Unknown,
// unconfigured, and empty integration IDs have no entry and therefore remain
// quarantined. Source-policy exact rules, trusted_sources entries, and
// source-policy patterns are parsed once into their documented precedence.
func ResolveTrustPolicy(cfg *config.Config) (TrustPolicy, error) {
	resolved, err := adapters.Resolve(cfg)
	if err != nil {
		return TrustPolicy{}, err
	}
	rules, err := cfg.SourcePolicyRules()
	if err != nil {
		return TrustPolicy{}, err
	}
	policy := TrustPolicy{
		integrationTrust: make(map[string]string, len(resolved)),
		trustedSources:   make(map[string]bool, len(cfg.Section("trusted_sources"))),
		exactPolicies:    map[string]string{},
	}
	for _, integration := range resolved {
		policy.integrationTrust[integration.Name] = integration.DefaultTrust
	}
	for source := range cfg.Section("trusted_sources") {
		policy.trustedSources[source] = true
	}
	for _, rule := range rules {
		if rule.Pattern {
			policy.patterns = append(policy.patterns, rule)
			continue
		}
		policy.exactPolicies[rule.Source] = rule.Trust
	}
	sort.SliceStable(policy.patterns, func(i, j int) bool {
		return len(policy.patterns[i].Source) > len(policy.patterns[j].Source)
	})
	return policy, nil
}

// Quarantined reports whether source is not allowed to enter normalisation.
// It first rejects malformed source namespaces. Valid captures then resolve in
// order: exact source_policies, exact trusted_sources, the most-specific
// source_policies pattern, the canonical human authority, configured integration
// default, then quarantine. No product-name inference is performed; `human` is
// the protocol's reserved authoritative principal rather than an integration.
func (p TrustPolicy) Quarantined(c Capture) bool {
	if !validCaptureSource(c) {
		return true
	}
	if trust, ok := p.exactPolicies[c.Source]; ok {
		return trust != "supervised"
	}
	if p.trustedSources[c.Source] {
		return false
	}
	for _, pattern := range p.patterns {
		if strings.HasPrefix(c.Source, pattern.Source) {
			return pattern.Trust != "supervised"
		}
	}
	if c.Agent == "human" {
		return false
	}
	return p.integrationTrust[c.Agent] != "supervised"
}

func validCaptureSource(c Capture) bool {
	agent, machine, ok := config.ParseSourceID(c.Source)
	return ok && agent == c.Agent && machine == c.Machine
}
