package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxManagedGroups = 64
	maxGroupRules    = 100
)

var errInvalidGroupMembership = errors.New("invalid selected node")

func (a *App) ensureManagedGroups() error {
	defaults := defaultManagedGroups()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for position, group := range defaults {
		encoded, err := json.Marshal(group)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO managed_groups(id,position,config_json) VALUES(?,?,?)`, group.ID, position, encoded); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *App) loadManagedGroups() ([]ManagedGroup, error) {
	rows, err := a.db.Query(`SELECT config_json FROM managed_groups ORDER BY position,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make([]ManagedGroup, 0)
	for rows.Next() {
		var encoded []byte
		var group ManagedGroup
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(encoded, &group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (a *App) listManagedGroups(w http.ResponseWriter) {
	groups, err := a.loadManagedGroups()
	if err != nil {
		internal(w)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"items": groups, "total": len(groups)})
}

func validManagedGroupName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 || strings.IndexFunc(name, unicode.IsControl) >= 0 || !validGroupName(name) {
		return "", false
	}
	return name, true
}

func validGroupTestURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func validateManagedGroups(groups []ManagedGroup) error {
	if len(groups) > maxManagedGroups {
		return errors.New("too many groups")
	}
	ids := make(map[string]bool, len(groups))
	names := make(map[string]bool, len(groups))
	defaults := make(map[string]bool)
	for _, group := range defaultManagedGroups() {
		defaults[group.ID] = true
	}
	for _, group := range groups {
		name, ok := validManagedGroupName(group.Name)
		if !ok || group.ID == "" || len(group.ID) > 80 || ids[group.ID] {
			return errors.New("invalid group identity")
		}
		key := strings.ToLower(name)
		if names[key] {
			return errors.New("duplicate group name")
		}
		ids[group.ID] = true
		names[key] = true
		if defaults[group.ID] && !group.BuiltIn {
			return errors.New("built-in group identity cannot change")
		}
		if group.ID == "general" && !group.Enabled {
			return errors.New("general group must remain enabled")
		}
		if group.NodeMode != "all_enabled" && group.NodeMode != "selected" {
			return errors.New("invalid membership mode")
		}
		if group.NodeMode == "all_enabled" && len(group.NodeIDs) != 0 {
			return errors.New("all-enabled groups cannot specify node ids")
		}
		if len(group.NodeIDs) > maxNodes {
			return errors.New("too many selected nodes")
		}
		seenNodes := make(map[string]bool, len(group.NodeIDs))
		for _, id := range group.NodeIDs {
			if id == "" || seenNodes[id] {
				return errors.New("invalid selected node")
			}
			seenNodes[id] = true
		}
		if group.Strategy != "url-test" {
			return errors.New("unsupported group strategy")
		}
		if !validGroupTestURL(group.TestURL) || group.IntervalSeconds < 1 || group.IntervalSeconds > 86400 || group.TimeoutMs < 0 || group.TimeoutMs > 120000 || (group.ExpectedStatus != "" && !validExpectedStatus(group.ExpectedStatus)) {
			return errors.New("invalid group test settings")
		}
		if len(group.Rules) > maxGroupRules {
			return errors.New("too many group rules")
		}
		seenRules := make(map[string]bool, len(group.Rules))
		for _, rule := range group.Rules {
			line, ok := renderGroupRule(rule, "managed-group-validation")
			if !ok || seenRules[line] {
				return errors.New("invalid group rule")
			}
			seenRules[line] = true
		}
		if group.ID == "general" && len(group.Rules) != 0 {
			return errors.New("general match group cannot have domain rules")
		}
	}
	for id := range defaults {
		if !ids[id] {
			return errors.New("built-in groups cannot be removed")
		}
	}
	return nil
}

func (a *App) validateSelectedNodeIDs(ids []string) error {
	if len(ids) > maxNodes {
		return errInvalidGroupMembership
	}
	if len(ids) == 0 {
		return nil
	}
	nodes, err := a.allNodes(false)
	if err != nil {
		return err
	}
	available := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		available[node.ID] = true
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] || !available[id] {
			return errInvalidGroupMembership
		}
		seen[id] = true
	}
	return nil
}

// persistManagedGroupsLocked atomically replaces the policy and public YAML.
// groupsMu must be held by the caller.
func (a *App) persistManagedGroupsLocked(groups []ManagedGroup) error {
	if err := validateManagedGroups(groups); err != nil {
		return err
	}
	nodes, err := a.allNodes(true)
	if err != nil {
		return err
	}
	usable := nodes[:0]
	for _, node := range nodes {
		if node.Enabled {
			usable = append(usable, node)
		}
	}
	publicConfig, err := buildManagedConfig(usable, groups)
	if err != nil {
		return err
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM managed_groups`); err != nil {
		return err
	}
	for position, group := range groups {
		encoded, err := json.Marshal(group)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO managed_groups(id,position,config_json) VALUES(?,?,?)`, group.ID, position, encoded); err != nil {
			return err
		}
	}
	var old []byte
	if err := tx.QueryRow(`SELECT config FROM output_settings WHERE id=1`).Scan(&old); err != nil && err != sql.ErrNoRows {
		return err
	}
	if !bytes.Equal(old, publicConfig) {
		if _, err := tx.Exec(`UPDATE output_settings SET config=?,generated_at=?,revision=revision+1 WHERE id=1`, publicConfig, stamp(time.Now().UTC())); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func groupValidationResponse(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "duplicate group name") {
		apiError(w, http.StatusConflict, "duplicate_name", "A group with this name already exists")
		return
	}
	apiError(w, http.StatusBadRequest, "invalid_group", "Group name or settings are invalid")
}

func (a *App) createManagedGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name            string      `json:"name"`
		Enabled         *bool       `json:"enabled"`
		Rules           []GroupRule `json:"rules"`
		NodeMode        string      `json:"nodeMode"`
		NodeIDs         []string    `json:"nodeIds"`
		Strategy        string      `json:"strategy"`
		TestURL         string      `json:"testUrl"`
		IntervalSeconds int         `json:"intervalSeconds"`
		TimeoutMs       int         `json:"timeoutMs"`
		ExpectedStatus  string      `json:"expectedStatus"`
		Lazy            *bool       `json:"lazy"`
	}
	if !decode(w, r, &in) {
		return
	}
	name, ok := validManagedGroupName(in.Name)
	if !ok {
		apiError(w, http.StatusBadRequest, "invalid_group", "Group name or settings are invalid")
		return
	}
	id, err := newID()
	if err != nil {
		internal(w)
		return
	}
	group := ManagedGroup{
		ID: id, Name: name, Enabled: true, Rules: in.Rules,
		NodeMode: in.NodeMode, NodeIDs: in.NodeIDs, Strategy: in.Strategy,
		TestURL: in.TestURL, IntervalSeconds: in.IntervalSeconds,
		TimeoutMs: in.TimeoutMs, ExpectedStatus: in.ExpectedStatus,
		Lazy: false, BuiltIn: false,
	}
	if in.Enabled != nil {
		group.Enabled = *in.Enabled
	}
	if in.Lazy != nil {
		group.Lazy = *in.Lazy
	}
	if group.NodeMode == "" {
		group.NodeMode = "all_enabled"
	}
	if group.Strategy == "" {
		group.Strategy = "url-test"
	}
	if group.TestURL == "" {
		group.TestURL = "https://www.gstatic.com/generate_204"
	}
	if group.IntervalSeconds == 0 {
		group.IntervalSeconds = 180
	}
	if group.TimeoutMs == 0 {
		group.TimeoutMs = 5000
	}
	if group.ExpectedStatus == "" {
		group.ExpectedStatus = "204"
	}
	if group.NodeIDs == nil {
		group.NodeIDs = []string{}
	}
	if group.Rules == nil {
		group.Rules = []GroupRule{}
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	groups, err := a.loadManagedGroups()
	if err != nil {
		internal(w)
		return
	}
	groups = append(groups, group)
	if err := a.validateSelectedNodeIDs(group.NodeIDs); err != nil {
		if errors.Is(err, errInvalidGroupMembership) {
			apiError(w, http.StatusBadRequest, "invalid_group", "Group name or settings are invalid")
		} else {
			internal(w)
		}
		return
	}
	if err := validateManagedGroups(groups); err != nil {
		groupValidationResponse(w, err)
		return
	}
	if err := a.persistManagedGroupsLocked(groups); err != nil {
		internal(w)
		return
	}
	jsonResponse(w, http.StatusCreated, map[string]interface{}{"group": group})
}

type managedGroupPatch struct {
	Name            *string      `json:"name"`
	Enabled         *bool        `json:"enabled"`
	Rules           *[]GroupRule `json:"rules"`
	NodeMode        *string      `json:"nodeMode"`
	NodeIDs         *[]string    `json:"nodeIds"`
	Strategy        *string      `json:"strategy"`
	TestURL         *string      `json:"testUrl"`
	IntervalSeconds *int         `json:"intervalSeconds"`
	TimeoutMs       *int         `json:"timeoutMs"`
	ExpectedStatus  *string      `json:"expectedStatus"`
	Lazy            *bool        `json:"lazy"`
}

func (a *App) patchManagedGroup(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" || strings.Contains(id, "/") {
		apiError(w, http.StatusNotFound, "not_found", "Group not found")
		return
	}
	var patch managedGroupPatch
	if !decode(w, r, &patch) {
		return
	}
	if patch.Name == nil && patch.Enabled == nil && patch.Rules == nil && patch.NodeMode == nil && patch.NodeIDs == nil && patch.Strategy == nil && patch.TestURL == nil && patch.IntervalSeconds == nil && patch.TimeoutMs == nil && patch.ExpectedStatus == nil && patch.Lazy == nil {
		apiError(w, http.StatusBadRequest, "invalid_group", "No group changes provided")
		return
	}
	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	groups, err := a.loadManagedGroups()
	if err != nil {
		internal(w)
		return
	}
	index := -1
	for i := range groups {
		if groups[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		apiError(w, http.StatusNotFound, "not_found", "Group not found")
		return
	}
	group := groups[index]
	if patch.Name != nil {
		group.Name = *patch.Name
	}
	if patch.Enabled != nil {
		group.Enabled = *patch.Enabled
	}
	if patch.Rules != nil {
		group.Rules = *patch.Rules
	}
	if patch.NodeMode != nil {
		group.NodeMode = *patch.NodeMode
	}
	if patch.NodeIDs != nil {
		group.NodeIDs = *patch.NodeIDs
	}
	if patch.Strategy != nil {
		group.Strategy = *patch.Strategy
	}
	if patch.TestURL != nil {
		group.TestURL = *patch.TestURL
	}
	if patch.IntervalSeconds != nil {
		group.IntervalSeconds = *patch.IntervalSeconds
	}
	if patch.TimeoutMs != nil {
		group.TimeoutMs = *patch.TimeoutMs
	}
	if patch.ExpectedStatus != nil {
		group.ExpectedStatus = *patch.ExpectedStatus
	}
	if patch.Lazy != nil {
		group.Lazy = *patch.Lazy
	}
	if group.NodeMode == "all_enabled" {
		group.NodeIDs = []string{}
	}
	if err := a.validateSelectedNodeIDs(group.NodeIDs); err != nil {
		if errors.Is(err, errInvalidGroupMembership) {
			apiError(w, http.StatusBadRequest, "invalid_group", "Group name or settings are invalid")
		} else {
			internal(w)
		}
		return
	}
	groups[index] = group
	if err := validateManagedGroups(groups); err != nil {
		groupValidationResponse(w, err)
		return
	}
	if err := a.persistManagedGroupsLocked(groups); err != nil {
		internal(w)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"group": group})
}

func (a *App) deleteManagedGroup(w http.ResponseWriter, id string) {
	if id == "" || strings.Contains(id, "/") {
		apiError(w, http.StatusNotFound, "not_found", "Group not found")
		return
	}
	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	groups, err := a.loadManagedGroups()
	if err != nil {
		internal(w)
		return
	}
	index := -1
	for i := range groups {
		if groups[i].ID == id {
			index = i
			if groups[i].BuiltIn {
				apiError(w, http.StatusConflict, "builtin_group", "Built-in groups cannot be deleted")
				return
			}
			break
		}
	}
	if index < 0 {
		apiError(w, http.StatusNotFound, "not_found", "Group not found")
		return
	}
	groups = append(groups[:index], groups[index+1:]...)
	if err := a.persistManagedGroupsLocked(groups); err != nil {
		internal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
