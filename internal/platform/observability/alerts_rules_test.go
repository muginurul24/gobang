package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type alertRuleFile struct {
	Groups []alertGroup `yaml:"groups"`
}

type alertGroup struct {
	Name  string      `yaml:"name"`
	Rules []alertRule `yaml:"rules"`
}

type alertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

func TestAlertRulesBaseline(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "monitoring", "alerts.rules.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read alert rules: %v", err)
	}

	var parsed alertRuleFile
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal alert rules: %v", err)
	}

	if len(parsed.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(parsed.Groups))
	}
	if parsed.Groups[0].Name != "onixggr-alerts" {
		t.Fatalf("group name = %q, want onixggr-alerts", parsed.Groups[0].Name)
	}

	expected := map[string]string{
		"OnixggrWebhookFailureSpike":    "onixggr_webhook_events_total",
		"OnixggrCallbackFailureSpike":   "onixggr_recent_failures",
		"OnixggrRedisDown":              "onixggr_dependency_up",
		"OnixggrDatabaseDown":           "onixggr_dependency_up",
		"OnixggrNexusGGRErrorSpike":     "onixggr_upstream_request_duration_seconds_count",
		"OnixggrQRISProviderErrorSpike": "onixggr_upstream_request_duration_seconds_count",
	}

	rulesByName := make(map[string]alertRule, len(parsed.Groups[0].Rules))
	for _, rule := range parsed.Groups[0].Rules {
		rulesByName[rule.Alert] = rule
	}

	if len(rulesByName) != len(expected) {
		t.Fatalf("rules = %d, want %d", len(rulesByName), len(expected))
	}

	for name, metric := range expected {
		rule, ok := rulesByName[name]
		if !ok {
			t.Fatalf("missing alert rule %q", name)
		}
		if strings.TrimSpace(rule.Expr) == "" {
			t.Fatalf("alert %q has empty expr", name)
		}
		if !strings.Contains(rule.Expr, metric) {
			t.Fatalf("alert %q expr = %q, want metric %q", name, rule.Expr, metric)
		}
		if strings.TrimSpace(rule.For) == "" {
			t.Fatalf("alert %q has empty for", name)
		}
		if rule.Labels["severity"] == "" {
			t.Fatalf("alert %q missing severity label", name)
		}
		if strings.TrimSpace(rule.Annotations["summary"]) == "" {
			t.Fatalf("alert %q missing summary annotation", name)
		}
	}
}
