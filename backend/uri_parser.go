package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type parseCodeError struct{ code string }

func (e *parseCodeError) Error() string { return e.code }

func parseFailure(code string) error { return &parseCodeError{code: code} }

const (
	maxSubscriptionLines = maxNodes * 2
	maxURIBytes          = 16 << 10
)

var supportedURISchemes = map[string]bool{
	"ss": true, "vmess": true, "vless": true, "trojan": true,
	"hysteria2": true, "hy2": true,
}

var supportedSSCiphers = map[string]bool{
	"aes-128-ctr": true, "aes-192-ctr": true, "aes-256-ctr": true,
	"aes-128-cfb": true, "aes-192-cfb": true, "aes-256-cfb": true,
	"aes-128-gcm": true, "aes-192-gcm": true, "aes-256-gcm": true,
	"aes-128-ccm": true, "aes-192-ccm": true, "aes-256-ccm": true,
	"aes-128-gcm-siv": true, "aes-256-gcm-siv": true,
	"chacha20-ietf": true, "chacha20": true, "xchacha20": true,
	"chacha20-ietf-poly1305": true, "xchacha20-ietf-poly1305": true,
	"chacha8-ietf-poly1305": true, "2022-blake3-aes-128-gcm": true,
	"2022-blake3-aes-256-gcm": true, "2022-blake3-chacha20-poly1305": true,
	"lea-128-gcm": true, "lea-192-gcm": true, "lea-256-gcm": true,
	"rabbit128-poly1305": true, "aegis-128l": true, "aegis-256": true,
	"aez-384": true, "deoxys-ii-256-128": true, "rc4-md5": true, "none": true,
}

// parseURIList reports handled=false unless the body clearly looks like a list
// of URI links. The error strings are stable internal codes and never contain
// the input URI or credential material.
func parseURIList(data []byte) ([]map[string]interface{}, bool, error) {
	if len(data) > maxFetchBytes {
		return nil, true, parseFailure("invalid_subscription")
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if text == "" {
		return nil, false, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxSubscriptionLines {
		return nil, true, parseFailure("invalid_subscription")
	}
	links := make([]string, 0, len(lines))
	looksLikeList := false
	nonURI := false
	unsupported := false
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > maxURIBytes {
			return nil, true, parseFailure("invalid_subscription")
		}
		sep := strings.Index(line, "://")
		if sep < 1 || !isScheme(line[:sep]) {
			nonURI = true
			continue
		}
		looksLikeList = true
		scheme := strings.ToLower(line[:sep])
		if !supportedURISchemes[scheme] {
			unsupported = true
		}
		links = append(links, line)
	}
	if !looksLikeList {
		return nil, false, nil
	}
	if nonURI {
		return nil, true, parseFailure("invalid_subscription")
	}
	if unsupported {
		return nil, true, parseFailure("unsupported_format")
	}
	if len(links) == 0 || len(links) > maxNodes {
		return nil, true, parseFailure("invalid_subscription")
	}
	proxies := make([]map[string]interface{}, 0, len(links))
	for _, link := range links {
		proxy, err := parseProxyURI(link)
		if err != nil {
			return nil, true, parseFailure("invalid_subscription")
		}
		proxies = append(proxies, proxy)
	}
	return proxies, true, nil
}

func isScheme(s string) bool {
	if s == "" || !((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func parseProxyURI(raw string) (map[string]interface{}, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid link")
	}
	switch strings.ToLower(u.Scheme) {
	case "ss":
		return parseSS(raw, u)
	case "vmess":
		return parseVMess(raw, u)
	case "vless":
		return parseVLESS(u)
	case "trojan":
		return parseTrojan(u)
	case "hysteria2", "hy2":
		return parseHysteria2(u)
	default:
		return nil, errors.New("unsupported link")
	}
}

func parseSS(raw string, parsed *url.URL) (map[string]interface{}, error) {
	main := strings.TrimPrefix(raw, raw[:strings.Index(raw, "://")+3])
	if i := strings.IndexAny(main, "?#"); i >= 0 {
		main = main[:i]
	}
	var credential, authority string
	if at := strings.LastIndex(main, "@"); at >= 0 {
		credential, authority = main[:at], main[at+1:]
	} else {
		decoded, ok := decodeLinkBase64(main)
		if !ok {
			return nil, errors.New("invalid ss link")
		}
		at := strings.LastIndex(decoded, "@")
		if at < 0 {
			return nil, errors.New("invalid ss authority")
		}
		credential, authority = decoded[:at], decoded[at+1:]
	}
	decodedCredential, ok := decodeLinkBase64(credential)
	if ok {
		credential = decodedCredential
	} else if decoded, err := url.PathUnescape(credential); err == nil {
		credential = decoded
	}
	colon := strings.IndexByte(credential, ':')
	if colon <= 0 || colon == len(credential)-1 {
		return nil, errors.New("invalid ss credentials")
	}
	method, password := strings.ToLower(credential[:colon]), credential[colon+1:]
	host, port, err := splitHostPort(authority)
	if err != nil || password == "" || !supportedSSCiphers[method] {
		return nil, errors.New("invalid ss endpoint")
	}
	name := uriName(parsed.Fragment, host, port)
	proxy := map[string]interface{}{"name": name, "type": "ss", "server": host, "port": port, "cipher": method, "password": password, "udp": true}
	query := parsed.Query()
	if plugin := query.Get("plugin"); plugin != "" {
		pluginName, opts, ok := parseSSPlugin(plugin)
		if !ok || pluginName == "" {
			return nil, errors.New("unsupported ss plugin")
		}
		proxy["plugin"] = pluginName
		if len(opts) > 0 {
			proxy["plugin-opts"] = opts
		}
	}
	return proxy, nil
}

func parseSSPlugin(value string) (string, map[string]interface{}, bool) {
	parts := strings.Split(value, ";")
	name := strings.TrimSpace(parts[0])
	opts := map[string]interface{}{}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "tls" {
			opts["tls"] = true
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			return "", nil, false
		}
		opts[key] = val
	}
	switch name {
	case "obfs-local":
		mode := firstNonEmpty(stringValue(opts["obfs"]), "http")
		if mode != "http" && mode != "tls" {
			return "", nil, false
		}
		mapped := map[string]interface{}{"mode": mode}
		if host := stringValue(opts["obfs-host"]); host != "" {
			mapped["host"] = host
		}
		return "obfs", mapped, true
	case "obfs":
		for key := range opts {
			if key != "mode" && key != "host" {
				return "", nil, false
			}
		}
		return name, opts, true
	case "v2ray-plugin":
		for key := range opts {
			if key != "mode" && key != "tls" && key != "host" && key != "path" && key != "mux" {
				return "", nil, false
			}
		}
		if opts["tls"] != nil && stringValue(opts["tls"]) == "true" {
			opts["tls"] = true
		}
		return name, opts, true
	default:
		return "", nil, false
	}
}

func parseVMess(raw string, u *url.URL) (map[string]interface{}, error) {
	payload := strings.TrimPrefix(raw, raw[:strings.Index(raw, "://")+3])
	if i := strings.IndexAny(payload, "?#"); i >= 0 {
		payload = payload[:i]
	}
	decoded, ok := decodeLinkBase64(payload)
	if !ok {
		return nil, errors.New("invalid vmess payload")
	}
	var value map[string]interface{}
	if json.Unmarshal([]byte(decoded), &value) != nil || value == nil {
		return nil, errors.New("invalid vmess json")
	}
	host := firstNonEmpty(stringValue(value["add"]), stringValue(value["server"]))
	port, err := parsePort(stringValue(value["port"]))
	id := firstNonEmpty(stringValue(value["id"]), stringValue(value["uuid"]))
	if err != nil || !validHost(host) || id == "" || len(id) > 256 {
		return nil, errors.New("invalid vmess endpoint")
	}
	name := uriName(u.Fragment, host, port)
	if u.Fragment == "" {
		name = uriName(stringValue(value["ps"]), host, port)
	}
	cipher := firstNonEmpty(stringValue(value["scy"]), stringValue(value["cipher"]), "auto")
	proxy := map[string]interface{}{"name": name, "type": "vmess", "server": host, "port": port, "uuid": id, "alterId": integerValue(value["aid"]), "cipher": cipher, "udp": true}
	network := firstNonEmpty(stringValue(value["net"]), "tcp")
	if !applyTransport(proxy, network, stringValue(value["path"]), stringValue(value["host"]), stringValue(value["serviceName"])) {
		return nil, errors.New("unsupported vmess network")
	}
	tls := truthy(value["tls"])
	if tls {
		proxy["tls"] = true
		setTLSFields(proxy, stringValue(value["sni"]), stringValue(value["alpn"]), stringValue(value["fp"]), truthy(value["allowInsecure"]))
	}
	return proxy, nil
}

func parseVLESS(u *url.URL) (map[string]interface{}, error) {
	host, port, err := uriHostPort(u)
	if err != nil || u.User == nil {
		return nil, errors.New("invalid vless endpoint")
	}
	id, _ := u.User.Password()
	if id == "" {
		id = u.User.Username()
	}
	if id == "" || len(id) > 256 {
		return nil, errors.New("invalid vless credentials")
	}
	q := u.Query()
	encryption := firstNonEmpty(q.Get("encryption"), "none")
	if encryption != "none" {
		return nil, errors.New("unsupported vless encryption")
	}
	proxy := map[string]interface{}{"name": uriName(u.Fragment, host, port), "type": "vless", "server": host, "port": port, "uuid": id, "udp": true, "encryption": encryption}
	security := strings.ToLower(q.Get("security"))
	if security == "tls" || security == "reality" || truthyString(q.Get("tls")) {
		proxy["tls"] = true
		setTLSFields(proxy, firstNonEmpty(q.Get("sni"), q.Get("serverName")), q.Get("alpn"), q.Get("fp"), queryBool(q, "allowInsecure", "insecure"))
	}
	if flow := q.Get("flow"); flow != "" {
		proxy["flow"] = flow
	}
	if security == "reality" {
		reality := map[string]interface{}{}
		key := firstNonEmpty(q.Get("pbk"), q.Get("publicKey"))
		if key == "" {
			return nil, errors.New("missing reality public key")
		}
		reality["public-key"] = key
		if sid := firstNonEmpty(q.Get("sid"), q.Get("shortId")); sid != "" {
			reality["short-id"] = sid
		}
		proxy["reality-opts"] = reality
	}
	network := firstNonEmpty(q.Get("type"), "tcp")
	if !applyTransport(proxy, network, q.Get("path"), firstNonEmpty(q.Get("host"), q.Get("authority")), firstNonEmpty(q.Get("serviceName"), q.Get("service_name"))) {
		return nil, errors.New("unsupported vless network")
	}
	return proxy, nil
}

func parseTrojan(u *url.URL) (map[string]interface{}, error) {
	host, port, err := uriHostPort(u)
	if err != nil || u.User == nil {
		return nil, errors.New("invalid trojan endpoint")
	}
	password, hasPassword := u.User.Password()
	if !hasPassword {
		password = u.User.Username()
	}
	if password == "" || len(password) > 2048 {
		return nil, errors.New("invalid trojan credentials")
	}
	q := u.Query()
	proxy := map[string]interface{}{"name": uriName(u.Fragment, host, port), "type": "trojan", "server": host, "port": port, "password": password, "udp": true}
	setTrojanTLSFields(proxy, firstNonEmpty(q.Get("sni"), q.Get("peer"), q.Get("serverName")), q.Get("alpn"), q.Get("fp"), queryBool(q, "allowInsecure", "insecure"))
	network := firstNonEmpty(q.Get("type"), "tcp")
	if !applyTransport(proxy, network, q.Get("path"), firstNonEmpty(q.Get("host"), q.Get("authority")), firstNonEmpty(q.Get("serviceName"), q.Get("service_name"))) {
		return nil, errors.New("unsupported trojan network")
	}
	return proxy, nil
}

func parseHysteria2(u *url.URL) (map[string]interface{}, error) {
	host, port, err := uriHostPort(u)
	if err != nil || u.User == nil {
		return nil, errors.New("invalid hysteria2 endpoint")
	}
	password, hasPassword := u.User.Password()
	if !hasPassword {
		password = u.User.Username()
	}
	if password == "" || len(password) > 2048 {
		return nil, errors.New("invalid hysteria2 credentials")
	}
	q := u.Query()
	proxy := map[string]interface{}{"name": uriName(u.Fragment, host, port), "type": "hysteria2", "server": host, "port": port, "password": password, "udp": true}
	setTrojanTLSFields(proxy, firstNonEmpty(q.Get("sni"), q.Get("serverName")), q.Get("alpn"), firstNonEmpty(q.Get("fp"), q.Get("fingerprint")), queryBool(q, "insecure", "allowInsecure"))
	if obfs := firstNonEmpty(q.Get("obfs"), q.Get("obfs-type")); obfs != "" {
		proxy["obfs"] = obfs
	}
	if obfsPassword := firstNonEmpty(q.Get("obfs-password"), q.Get("obfs_password")); obfsPassword != "" {
		proxy["obfs-password"] = obfsPassword
	}
	for _, key := range []string{"up", "down", "hop-interval", "ports"} {
		if val := q.Get(key); val != "" {
			proxy[key] = val
		}
	}
	return proxy, nil
}

func uriHostPort(u *url.URL) (string, int, error) {
	if u == nil || u.Hostname() == "" || u.Port() == "" {
		return "", 0, errors.New("missing endpoint")
	}
	port, err := parsePort(u.Port())
	if err != nil || !validHost(u.Hostname()) {
		return "", 0, errors.New("invalid endpoint")
	}
	return u.Hostname(), port, nil
}

func splitHostPort(value string) (string, int, error) {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || !validHost(host) {
		return "", 0, errors.New("invalid authority")
	}
	port, err := parsePort(portText)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func validHost(host string) bool {
	return host != "" && len(host) <= 253 && !strings.ContainsAny(host, " \t\r\n")
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("invalid port")
	}
	return port, nil
}

func decodeLinkBase64(value string) (string, bool) {
	value = strings.TrimSpace(value)
	encodings := []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding}
	for _, enc := range encodings {
		if b, err := enc.DecodeString(value); err == nil && len(b) > 0 {
			return string(b), true
		}
	}
	if decoded, err := url.PathUnescape(value); err == nil && decoded != value {
		for _, enc := range encodings {
			if b, err := enc.DecodeString(decoded); err == nil && len(b) > 0 {
				return string(b), true
			}
		}
	}
	return "", false
}

func decodeSubscriptionBase64(data []byte) []byte {
	value := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(string(data)))
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := enc.DecodeString(value); err == nil && len(decoded) > 0 {
			return decoded
		}
	}
	return nil
}

func uriName(fragment, host string, port int) string {
	fragment = strings.TrimSpace(fragment)
	if fragment != "" && len(fragment) <= maxNameLength && !strings.ContainsAny(fragment, "\r\n\x00") {
		return fragment
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func stringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

func integerValue(value interface{}) int {
	n, _ := strconv.Atoi(stringValue(value))
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truthy(value interface{}) bool { return truthyString(stringValue(value)) }

func truthyString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "tls", "yes", "on":
		return true
	default:
		return false
	}
}

func queryBool(q url.Values, keys ...string) bool {
	for _, key := range keys {
		if q.Has(key) {
			return truthyString(q.Get(key))
		}
	}
	return false
}

func splitALPN(value string) []interface{} {
	out := []interface{}{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func setTLSFields(proxy map[string]interface{}, sni, alpn, fingerprint string, insecure bool) {
	if sni != "" {
		proxy["servername"] = sni
	}
	if alpn != "" {
		proxy["alpn"] = splitALPN(alpn)
	}
	if fingerprint != "" {
		proxy["client-fingerprint"] = fingerprint
	}
	if insecure {
		proxy["skip-cert-verify"] = true
	}
}

func setTrojanTLSFields(proxy map[string]interface{}, sni, alpn, fingerprint string, insecure bool) {
	if sni != "" {
		proxy["sni"] = sni
	}
	if alpn != "" {
		proxy["alpn"] = splitALPN(alpn)
	}
	if fingerprint != "" {
		proxy["client-fingerprint"] = fingerprint
	}
	if insecure {
		proxy["skip-cert-verify"] = true
	}
}

func applyTransport(proxy map[string]interface{}, network, path, host, service string) bool {
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" || network == "tcp" {
		return true
	}
	proxyType, _ := proxy["type"].(string)
	switch network {
	case "ws", "websocket":
		proxy["network"] = "ws"
		ws := map[string]interface{}{}
		if path != "" {
			ws["path"] = path
		}
		if host != "" {
			ws["headers"] = map[string]interface{}{"Host": host}
		}
		proxy["ws-opts"] = ws
		return true
	case "grpc":
		proxy["network"] = "grpc"
		if service != "" {
			proxy["grpc-opts"] = map[string]interface{}{"grpc-service-name": service}
		}
		return true
	case "http":
		if proxyType == "trojan" {
			return false
		}
		proxy["network"] = "http"
		opts := map[string]interface{}{}
		if path != "" {
			opts["path"] = []interface{}{path}
		}
		if host != "" {
			opts["headers"] = map[string]interface{}{"Host": []interface{}{host}}
		}
		if len(opts) > 0 {
			proxy["http-opts"] = opts
		}
		return true
	case "h2":
		if proxyType != "vmess" && proxyType != "vless" {
			return false
		}
		proxy["network"] = "h2"
		opts := map[string]interface{}{}
		if path != "" {
			opts["path"] = path
		}
		if host != "" {
			opts["host"] = []interface{}{host}
		}
		if len(opts) > 0 {
			proxy["h2-opts"] = opts
		}
		return true
	case "xhttp":
		if proxyType != "vless" {
			return false
		}
		proxy["network"] = "xhttp"
		opts := map[string]interface{}{}
		if path != "" {
			opts["path"] = path
		}
		if host != "" {
			opts["host"] = host
		}
		if len(opts) > 0 {
			proxy["xhttp-opts"] = opts
		}
		return true
	default:
		return false
	}
}
