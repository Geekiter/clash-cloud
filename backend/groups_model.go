package main

// GroupRule is a domain routing rule owned by a managed proxy group.
// Values are bare geosite names (for example "openai") or domain suffixes.
type GroupRule struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ManagedGroup describes a proxy group in the public unified profile.
// NodeIDs contains merged-node IDs and is only used when NodeMode is selected.
type ManagedGroup struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Enabled         bool        `json:"enabled"`
	Rules           []GroupRule `json:"rules"`
	NodeMode        string      `json:"nodeMode"`
	NodeIDs         []string    `json:"nodeIds"`
	Strategy        string      `json:"strategy"`
	TestURL         string      `json:"testUrl"`
	IntervalSeconds int         `json:"intervalSeconds"`
	TimeoutMs       int         `json:"timeoutMs"`
	ExpectedStatus  string      `json:"expectedStatus"`
	Lazy            bool        `json:"lazy"`
	BuiltIn         bool        `json:"builtIn"`
}

// defaultManagedGroups returns the stable initial policy set. The region
// groups intentionally have no domain routes until an operator adds rules.
func defaultManagedGroups() []ManagedGroup {
	service := func(id, name, site, testURL, status string) ManagedGroup {
		return ManagedGroup{
			ID: id, Name: name, Enabled: true,
			Rules:    []GroupRule{{Type: "geosite", Value: site}},
			NodeMode: "all_enabled", NodeIDs: []string{}, Strategy: "url-test", TestURL: testURL,
			IntervalSeconds: 180, TimeoutMs: 5000, ExpectedStatus: status,
			Lazy: false, BuiltIn: true,
		}
	}
	selected := func(id, name, testURL, expectedStatus string, rules []GroupRule) ManagedGroup {
		return ManagedGroup{
			ID: id, Name: name, Enabled: true, Rules: rules,
			NodeMode: "selected", NodeIDs: []string{}, Strategy: "url-test",
			TestURL: testURL, IntervalSeconds: 180,
			TimeoutMs: 5000, ExpectedStatus: expectedStatus, Lazy: false, BuiltIn: true,
		}
	}

	return []ManagedGroup{
		service("chatgpt", "ChatGPT", "openai", "https://chatgpt.com/", "200-399"),
		service("claude", "Claude", "anthropic", "https://claude.ai/", "200-399"),
		selected("google-us", "Google-US", "https://www.google.com/generate_204", "204", []GroupRule{{Type: "geosite", Value: "google"}, {Type: "geosite", Value: "youtube"}}),
		service("github", "GitHub", "github", "https://github.com/", "200-399"),
		service("reddit", "Reddit", "reddit", "https://www.reddit.com/", "200-399"),
		{
			ID: "general", Name: "通用", Enabled: true, Rules: []GroupRule{},
			NodeMode: "all_enabled", NodeIDs: []string{}, Strategy: "url-test",
			TestURL: "https://www.gstatic.com/generate_204", IntervalSeconds: 180,
			TimeoutMs: 5000, ExpectedStatus: "204", Lazy: false, BuiltIn: true,
		},
		selected("europe-us", "美欧", "https://www.gstatic.com/generate_204", "204", []GroupRule{}),
		selected("asia", "亚洲", "https://www.gstatic.com/generate_204", "204", []GroupRule{}),
	}
}
