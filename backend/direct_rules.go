package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const maxDirectDomains = 100

type DirectRoutingSettings struct {
	ChinaDomainDirect bool     `json:"chinaDomainDirect"`
	ChinaIPDirect     bool     `json:"chinaIPDirect"`
	Domains           []string `json:"domains"`
}

type directRoutingPatch struct {
	ChinaDomainDirect *bool     `json:"chinaDomainDirect"`
	ChinaIPDirect     *bool     `json:"chinaIPDirect"`
	Domains           *[]string `json:"domains"`
}

func defaultDirectRoutingSettings() DirectRoutingSettings {
	return DirectRoutingSettings{ChinaDomainDirect: true, ChinaIPDirect: true, Domains: []string{}}
}

func (a *App) ensureDirectRoutingSettings() error {
	encoded, err := json.Marshal(defaultDirectRoutingSettings())
	if err != nil {
		return err
	}
	_, err = a.db.Exec(`INSERT OR IGNORE INTO direct_routing_settings(id,config_json) VALUES(1,?)`, encoded)
	return err
}

func (a *App) loadDirectRoutingSettings() (DirectRoutingSettings, error) {
	var encoded []byte
	var settings DirectRoutingSettings
	err := a.db.QueryRow(`SELECT config_json FROM direct_routing_settings WHERE id=1`).Scan(&encoded)
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(encoded, &settings); err != nil {
		return settings, err
	}
	if settings.Domains == nil {
		settings.Domains = []string{}
	}
	return settings, nil
}

func normalizeDirectRoutingSettings(settings DirectRoutingSettings) (DirectRoutingSettings, bool) {
	if len(settings.Domains) > maxDirectDomains {
		return settings, false
	}
	seen := make(map[string]bool, len(settings.Domains))
	domains := make([]string, 0, len(settings.Domains))
	for _, value := range settings.Domains {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.TrimPrefix(value, ".")
		if value == "" || len(value) > 253 || !domainValuePattern.MatchString(value) || strings.Contains(value, "..") || seen[value] {
			return settings, false
		}
		seen[value] = true
		domains = append(domains, value)
	}
	settings.Domains = domains
	return settings, true
}

func (a *App) getDirectRoutingSettings(w http.ResponseWriter) {
	settings, err := a.loadDirectRoutingSettings()
	if err != nil {
		internal(w)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"settings": settings})
}

func (a *App) patchDirectRoutingSettings(w http.ResponseWriter, r *http.Request) {
	var patch directRoutingPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&patch); err != nil {
		apiError(w, http.StatusBadRequest, "invalid_direct_rules", "Direct routing settings are invalid")
		return
	}
	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	settings, err := a.loadDirectRoutingSettings()
	if err != nil {
		internal(w)
		return
	}
	if patch.ChinaDomainDirect != nil {
		settings.ChinaDomainDirect = *patch.ChinaDomainDirect
	}
	if patch.ChinaIPDirect != nil {
		settings.ChinaIPDirect = *patch.ChinaIPDirect
	}
	if patch.Domains != nil {
		settings.Domains = *patch.Domains
	}
	settings, ok := normalizeDirectRoutingSettings(settings)
	if !ok {
		apiError(w, http.StatusBadRequest, "invalid_direct_rules", "Direct routing settings are invalid")
		return
	}
	if err := a.persistDirectRoutingSettingsLocked(settings); err != nil {
		internal(w)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"settings": settings})
}

// persistDirectRoutingSettingsLocked persists the direct rules and generated
// client profile in the same transaction. groupsMu must be held by the caller.
func (a *App) persistDirectRoutingSettingsLocked(settings DirectRoutingSettings) error {
	settings, ok := normalizeDirectRoutingSettings(settings)
	if !ok {
		return errors.New("invalid direct routing settings")
	}
	groups, err := a.loadManagedGroups()
	if err != nil {
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
	publicConfig, err := buildManagedConfig(usable, groups, settings)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE direct_routing_settings SET config_json=? WHERE id=1`, encoded); err != nil {
		return err
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
