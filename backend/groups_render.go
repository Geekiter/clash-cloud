package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	geositeValuePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	domainValuePattern  = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
)

// buildManagedConfig renders the client-facing unified profile. It consumes
// only the supplied enabled-node snapshot and never contacts a controller.
func buildManagedConfig(nodes []Node, groups []ManagedGroup, direct DirectRoutingSettings) ([]byte, error) {
	proxies := make([]interface{}, 0, len(nodes))
	proxyNames := make(map[string]bool, len(nodes))
	nodeNames := make(map[string]string, len(nodes)) // merged-node ID -> proxy name
	for _, node := range nodes {
		if node.Raw == nil || strings.TrimSpace(node.Name) == "" || node.Name == "DIRECT" || node.Name == "MATCH" {
			continue
		}
		// Proxy names must be unique in Clash. Keep the first snapshot entry so
		// that group membership cannot create an ambiguous reference.
		if proxyNames[node.Name] {
			continue
		}
		proxy := make(map[string]interface{}, len(node.Raw)+1)
		for key, value := range node.Raw {
			proxy[key] = value
		}
		proxy["name"] = node.Name
		proxies = append(proxies, proxy)
		proxyNames[node.Name] = true
		nodeNames[node.ID] = node.Name
	}

	// Without any enabled proxy, publish a valid and deliberately direct-only
	// config. Do not leave dangling group or route references in the profile.
	if len(proxies) == 0 {
		return marshalManagedConfig(proxies, []interface{}{}, []string{"MATCH,DIRECT"})
	}

	proxyGroups := make([]interface{}, 0, len(groups))
	groupNames := make(map[string]bool, len(groups))
	groupByID := make(map[string]string, len(groups))
	generalIndex := -1
	for i, group := range groups {
		if group.ID == "general" {
			generalIndex = i
		}
	}

	// Region and service groups can have their membership changed independently.
	// Omit groups with no eligible members and later omit their routes as well.
	for _, group := range groups {
		if !group.Enabled || !validGroupName(group.Name) || group.ID == "" {
			continue
		}
		if group.Name == "DIRECT" || group.Name == "MATCH" || proxyNames[group.Name] || groupNames[group.Name] {
			continue
		}
		if group.NodeMode != "all_enabled" && group.NodeMode != "selected" {
			continue
		}
		members := make([]string, 0, len(nodeNames))
		if group.NodeMode == "all_enabled" {
			for _, node := range nodes {
				if node.Raw != nil && nodeNames[node.ID] != "" {
					members = appendUnique(members, nodeNames[node.ID])
				}
			}
		} else {
			for _, id := range group.NodeIDs {
				if name := nodeNames[id]; name != "" {
					members = appendUnique(members, name)
				}
			}
		}
		if len(members) == 0 {
			continue
		}
		strategy := group.Strategy
		if strategy == "" {
			strategy = "url-test"
		}
		switch strategy {
		case "url-test", "fallback", "load-balance", "select":
		default:
			return nil, fmt.Errorf("unsupported proxy group strategy %q", strategy)
		}

		entry := map[string]interface{}{
			"name": group.Name, "type": strategy, "proxies": members,
		}
		if strategy != "select" {
			testURL := strings.TrimSpace(group.TestURL)
			if testURL == "" {
				testURL = "https://www.gstatic.com/generate_204"
			}
			parsed, err := url.ParseRequestURI(testURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return nil, fmt.Errorf("invalid test URL for group %q", group.Name)
			}
			interval := group.IntervalSeconds
			if interval <= 0 {
				interval = 180
			}
			entry["url"] = testURL
			entry["interval"] = interval
			if group.TimeoutMs > 0 {
				entry["timeout"] = group.TimeoutMs
			}
			if expected := strings.TrimSpace(group.ExpectedStatus); expected != "" {
				if !validExpectedStatus(expected) {
					return nil, fmt.Errorf("invalid expected status for group %q", group.Name)
				}
				entry["expected-status"] = expected
			}
			entry["lazy"] = group.Lazy
		}
		proxyGroups = append(proxyGroups, entry)
		groupNames[group.Name] = true
		groupByID[group.ID] = group.Name
	}

	rules := make([]string, 0, len(groups)*2+len(direct.Domains)+3)
	for i, group := range groups {
		if i == generalIndex || group.ID == "general" {
			continue
		}
		name, emitted := groupByID[group.ID]
		if !emitted {
			continue
		}
		for _, rule := range group.Rules {
			line, ok := renderGroupRule(rule, name)
			if ok {
				rules = append(rules, line)
			}
		}
	}

	for _, domain := range direct.Domains {
		if normalized, ok := normalizeDirectRoutingSettings(DirectRoutingSettings{Domains: []string{domain}}); ok && len(normalized.Domains) == 1 {
			rules = append(rules, "DOMAIN-SUFFIX,"+normalized.Domains[0]+",DIRECT")
		}
	}
	if direct.ChinaDomainDirect {
		rules = append(rules, "GEOSITE,CN,DIRECT")
	}
	if direct.ChinaIPDirect {
		rules = append(rules, "GEOIP,CN,DIRECT")
	}

	// Only the reserved built-in ID owns the catch-all route. This prevents a
	// custom group name or user rule from changing MATCH semantics.
	if generalIndex >= 0 {
		if generalName := groupByID[groups[generalIndex].ID]; generalName != "" {
			rules = append(rules, "MATCH,"+generalName)
		} else {
			rules = append(rules, "MATCH,DIRECT")
		}
	} else {
		rules = append(rules, "MATCH,DIRECT")
	}
	return marshalManagedConfig(proxies, proxyGroups, rules)
}

func marshalManagedConfig(proxies, proxyGroups []interface{}, rules []string) ([]byte, error) {
	if rules == nil {
		rules = []string{"MATCH,DIRECT"}
	}
	config := map[string]interface{}{
		"mixed-port": 7890, "allow-lan": false, "mode": "rule", "log-level": "info",
		"proxies": proxies, "proxy-groups": proxyGroups, "rules": rules,
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, errors.New("cannot render managed config")
	}
	return data, nil
}

func renderGroupRule(rule GroupRule, groupName string) (string, bool) {
	value := strings.TrimSpace(rule.Value)
	if value == "" || strings.ContainsAny(value, ",\r\n\t ") {
		return "", false
	}
	switch rule.Type {
	case "geosite":
		if !geositeValuePattern.MatchString(value) {
			return "", false
		}
		return "GEOSITE," + value + "," + groupName, true
	case "domain-suffix":
		value = strings.TrimPrefix(value, ".")
		if !domainValuePattern.MatchString(value) || strings.Contains(value, "..") {
			return "", false
		}
		return "DOMAIN-SUFFIX," + value + "," + groupName, true
	default:
		return "", false
	}
}

func validExpectedStatus(value string) bool {
	for _, part := range strings.Split(value, "/") {
		bounds := strings.Split(part, "-")
		if len(bounds) > 2 {
			return false
		}
		low, err := strconv.Atoi(bounds[0])
		if err != nil || low < 100 || low > 599 {
			return false
		}
		if len(bounds) == 2 {
			high, err := strconv.Atoi(bounds[1])
			if err != nil || high < low || high > 599 {
				return false
			}
		}
	}
	return value != ""
}

func validGroupName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.ContainsAny(name, ",\r\n\t ")
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
