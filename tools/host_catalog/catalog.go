package host_catalog

import (
	_ "embed"
	"encoding/json"
	"errors"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tunnelbypass/core/installer"
	"tunnelbypass/internal/utils"
)

// TLS/SNI catalog from embedded hosts.json.
var mergedDefaultHosts []string

// Reality "dest" subset from embedded JSON.
var realityDestHosts []string

//go:embed hosts.json
var hostsByCategoryJSON []byte

type hostsByCategoryFile struct {
	RealityDest   []string            `json:"reality_dest_hosts"`
	CategoriesMap map[string][]string `json:"categories"`
}

// seedHostToCategory is the default classification map from the embedded JSON.
var seedHostToCategory = map[string]string{}

// catalogCategoryMergeOrder defines deterministic precedence when a host appears in multiple
// category lists in JSON. Country-scoped lists (e.g. egypt) must win over "general".
var catalogCategoryMergeOrder = []string{
	"gaming",
	"social",
	"streaming",
	"government-edu",
	"tech",
	"egypt",
	"general",
}

// Categories pinned out of the General UI bucket.
var countryCatalogCategories = map[string]bool{
	"egypt": true,
}

func init() {
	var f hostsByCategoryFile
	if err := json.Unmarshal(hostsByCategoryJSON, &f); err != nil {
		return
	}

	realityDestHosts = uniqueHostsOrdered(f.RealityDest)

	seedHostToCategory = map[string]string{}
	var allHosts []string

	seenCat := map[string]bool{}
	addHostsForCategory := func(cat string, hosts []string) {
		for _, h := range hosts {
			nh := NormalizeHost(h)
			if nh == "" {
				continue
			}
			allHosts = append(allHosts, nh)
			if _, exists := seedHostToCategory[nh]; !exists {
				seedHostToCategory[nh] = cat
			}
		}
	}

	for _, cat := range catalogCategoryMergeOrder {
		if hosts, ok := f.CategoriesMap[cat]; ok {
			addHostsForCategory(cat, hosts)
			seenCat[cat] = true
		}
	}
	for cat, hosts := range f.CategoriesMap {
		if seenCat[cat] {
			continue
		}
		addHostsForCategory(cat, hosts)
		seenCat[cat] = true
	}

	mergedDefaultHosts = uniqueHosts(allHosts)
}

type catalogFile struct {
	Hosts []string `json:"hosts"`
}

func catalogPath() string {
	return filepath.Join(installer.GetConfigDir("catalog"), "hosts.json")
}

// NormalizeHost returns a hostname suitable for SNI from a bare host, a full URL (https://…),
// or pasted host/path. Scheme, path, query, fragment, and port are stripped.
func NormalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	h = strings.ToLower(h)
	if strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") {
		u, err := url.Parse(h)
		if err == nil && u.Host != "" {
			h = u.Host
		} else {
			h = strings.TrimPrefix(h, "https://")
			h = strings.TrimPrefix(h, "http://")
			if i := strings.Index(h, "/"); i >= 0 {
				h = h[:i]
			}
			if i := strings.Index(h, "?"); i >= 0 {
				h = h[:i]
			}
			if i := strings.Index(h, "#"); i >= 0 {
				h = h[:i]
			}
		}
	} else {
		if i := strings.Index(h, "/"); i >= 0 {
			h = h[:i]
		}
		if i := strings.Index(h, "?"); i >= 0 {
			h = h[:i]
		}
		if i := strings.Index(h, "#"); i >= 0 {
			h = h[:i]
		}
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.TrimSpace(h)
}

func uniqueHosts(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range in {
		n := NormalizeHost(h)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// uniqueHostsOrdered deduplicates while preserving first-seen order (used for reality_dest_hosts).
func uniqueHostsOrdered(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range in {
		n := NormalizeHost(h)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func saveHosts(hosts []string) error {
	hosts = uniqueHosts(hosts)
	p := catalogPath()
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	data, _ := json.MarshalIndent(catalogFile{Hosts: hosts}, "", "  ")
	return os.WriteFile(p, data, 0644)
}

func loadHosts() ([]string, error) {
	p := catalogPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	data = utils.StripUTF8BOM(data)
	var f catalogFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return uniqueHosts(f.Hosts), nil
}

// Catalog is kept for backward compatibility; all categories resolve to the shared list.
type Catalog struct{}

// Legacy catalog handle; categories map to the shared list.
func NewCatalog() *Catalog {
	return &Catalog{}
}

// Copy of shared SNI/host list for menus and configs.
func DefaultHosts() []string {
	hosts, err := loadHosts()
	if err != nil || len(hosts) == 0 {
		hosts = uniqueHosts(mergedDefaultHosts)
		_ = saveHosts(hosts)
	}
	out := make([]string, len(hosts))
	copy(out, hosts)
	return out
}

// AddHost appends a host to the persistent JSON catalog.
func AddHost(host string) (bool, error) {
	h := NormalizeHost(host)
	if h == "" {
		return false, nil
	}
	hosts := DefaultHosts()
	for _, v := range hosts {
		if v == h {
			return false, nil
		}
	}
	hosts = append(hosts, h)
	return true, saveHosts(hosts)
}

// RemoveHost removes a host from the persistent JSON catalog.
func RemoveHost(host string) (bool, error) {
	h := NormalizeHost(host)
	if h == "" {
		return false, nil
	}
	hosts := DefaultHosts()
	var out []string
	removed := false
	for _, v := range hosts {
		if v == h {
			removed = true
			continue
		}
		out = append(out, v)
	}
	if !removed {
		return false, nil
	}
	return true, saveHosts(out)
}

// Domains aliases DefaultHosts for callers that used the old field name.
func (c *Catalog) Domains() []string {
	return DefaultHosts()
}

// GetRandomHost returns a random host; category is ignored (legacy API).
func (c *Catalog) GetRandomHost(category string) string {
	_ = category
	return RandomHost()
}

// All default hosts; category ignored (legacy).
func (c *Catalog) GetAllHosts(category string) []string {
	_ = category
	return DefaultHosts()
}

// RandomHost picks any catalog host.
func RandomHost() string {
	h := mergedDefaultHosts
	if len(h) == 0 {
		return ""
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return h[r.Intn(len(h))]
}

// RandomRealityDestHost picks a random host from the effective Reality dest pool.
func RandomRealityDestHost() string {
	h := EffectiveRealityDestHosts()
	if len(h) == 0 {
		return "www.facebook.com"
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return h[r.Intn(len(h))]
}

type realityDestPrefs struct {
	PreferredHost string   `json:"preferred_host,omitempty"`
	ExtraHosts    []string `json:"extra_hosts,omitempty"`
}

func prefsPath() string {
	return filepath.Join(installer.GetConfigDir("catalog"), "reality_dest_prefs.json")
}

func loadPrefs() (realityDestPrefs, error) {
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return realityDestPrefs{}, nil
		}
		return realityDestPrefs{}, err
	}
	data = utils.StripUTF8BOM(data)
	var p realityDestPrefs
	if err := json.Unmarshal(data, &p); err != nil {
		return realityDestPrefs{}, err
	}
	return p, nil
}

func savePrefs(p realityDestPrefs) error {
	_ = os.MkdirAll(filepath.Dir(prefsPath()), 0755)
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(prefsPath(), data, 0644)
}

// EffectiveRealityDestHosts returns embedded reality_dest_hosts (order preserved) plus any user-added
// extras from Diagnostic Tools. Used for Reality serverNames / Hysteria SNI and as the dest pool.
func EffectiveRealityDestHosts() []string {
	base := append([]string(nil), realityDestHosts...)
	p, err := loadPrefs()
	if err != nil {
		p = realityDestPrefs{}
	}
	seen := map[string]bool{}
	var out []string
	for _, h := range base {
		n := NormalizeHost(h)
		if n == "" {
			continue
		}
		k := strings.ToLower(n)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	for _, h := range p.ExtraHosts {
		n := NormalizeHost(h)
		if n == "" {
			continue
		}
		k := strings.ToLower(n)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	if len(out) == 0 {
		return []string{"www.facebook.com"}
	}
	return out
}

// PreferredRealityDestHost is the hostname used for Reality TCP dest and Hysteria masquerade when no
// tunnel SNI is set. Defaults to the first entry in EffectiveRealityDestHosts unless the user chose
// another in Diagnostic Tools (reality_dest_prefs.json).
func PreferredRealityDestHost() string {
	p, err := loadPrefs()
	if err == nil && strings.TrimSpace(p.PreferredHost) != "" {
		cand := NormalizeHost(p.PreferredHost)
		if cand != "" {
			return cand
		}
	}
	eff := EffectiveRealityDestHosts()
	if len(eff) > 0 {
		return eff[0]
	}
	return "www.facebook.com"
}

// DefaultRealityDestAddress is the TCP target for Xray REALITY "dest" and provision defaults.
// Hostnames in reality_dest_hosts become serverNames; some hosts map to a fixed IP (e.g. one.one.one.one → 1.1.1.1).
func DefaultRealityDestAddress() string {
	return realityTCPDestAddress(PreferredRealityDestHost())
}

func realityTCPDestAddress(host string) string {
	h := NormalizeHost(host)
	if h == "" {
		return "www.facebook.com:443"
	}
	if strings.EqualFold(h, "one.one.one.one") {
		return "1.1.1.1:443"
	}
	return h + ":443"
}

// SetPreferredRealityDestHost persists the user's choice; empty s clears preference (use first in list).
func SetPreferredRealityDestHost(host string) error {
	p, err := loadPrefs()
	if err != nil {
		p = realityDestPrefs{}
	}
	p.PreferredHost = NormalizeHost(strings.TrimSpace(host))
	return savePrefs(p)
}

// AddRealityDestExtraHost appends a hostname to the user dest pool (shown in serverNames + dest list).
func AddRealityDestExtraHost(host string) error {
	n := NormalizeHost(host)
	if n == "" {
		return errors.New("invalid hostname")
	}
	p, err := loadPrefs()
	if err != nil {
		p = realityDestPrefs{}
	}
	for _, e := range p.ExtraHosts {
		if strings.EqualFold(e, n) {
			return nil
		}
	}
	p.ExtraHosts = append(p.ExtraHosts, n)
	return savePrefs(p)
}

// ClearExtraRealityDestHosts removes user-added dest hosts (embedded list unchanged).
func ClearExtraRealityDestHosts() error {
	p, err := loadPrefs()
	if err != nil {
		p = realityDestPrefs{}
	}
	p.ExtraHosts = nil
	return savePrefs(p)
}

// IsRealityDestExtraHost reports whether h came from user prefs (not embedded hosts.json).
func IsRealityDestExtraHost(h string) bool {
	n := NormalizeHost(h)
	if n == "" {
		return false
	}
	p, err := loadPrefs()
	if err != nil {
		return false
	}
	for _, e := range p.ExtraHosts {
		if strings.EqualFold(NormalizeHost(e), n) {
			return true
		}
	}
	return false
}

// Default hosts minus primary (ExtraSNIs / serverNames).
func ExtraHostsExcluding(primary string) []string {
	var out []string
	for _, d := range mergedDefaultHosts {
		if d != primary {
			out = append(out, d)
		}
	}
	return out
}

// SNIsForSharing builds an ordered, deduplicated list: primary, then extras, then catalog.
func SNIsForSharing(primary string, extras []string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(primary)
	for _, e := range extras {
		add(e)
	}
	for _, h := range mergedDefaultHosts {
		add(h)
	}
	return out
}

// RealitySharingSNIs returns SharingLinkSNIs, or one default host when empty (same bootstrap as Hysteria).
func RealitySharingSNIs(primary string, extras []string) []string {
	sharing := SharingLinkSNIs(primary, extras)
	if len(sharing) == 0 {
		masq := strings.TrimSpace(primary)
		if masq == "" {
			masq = FirstRealityDestHost()
		}
		sharing = []string{NormalizeHost(masq)}
	}
	return sharing
}

// ServerNamesForVLESS builds Xray Reality serverNames: RealitySharingSNIs + reality_dest_hosts from hosts.json.
func ServerNamesForVLESS(primary string, extras []string) []string {
	return AppendRealityDestHosts(RealitySharingSNIs(primary, extras))
}

// AppendRealityDestHosts returns sharing (same list as SharingLinkSNIs / RealitySharingSNIs) plus
// mandatory Reality dest hosts from hosts.json (reality_dest_hosts), deduplicated. Order: sharing first,
// then dest. Use for Hysteria tls.sni, Xray Reality serverNames, and metadata — never mergedDefaultHosts.
func AppendRealityDestHosts(sharing []string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		h := NormalizeHost(s)
		if h == "" {
			return
		}
		k := strings.ToLower(h)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, h)
	}
	for _, h := range sharing {
		add(h)
	}
	destPool := EffectiveRealityDestHosts()
	for _, h := range destPool {
		add(h)
	}
	if len(out) == 0 {
		for _, h := range destPool {
			add(h)
		}
	}
	if len(out) == 0 {
		add("www.facebook.com")
		add("m.facebook.com")
	}
	return out
}

// FirstRealityDestHost is an alias for PreferredRealityDestHost (masquerade / cert CN).
func FirstRealityDestHost() string {
	return PreferredRealityDestHost()
}

// SharingLinkSNIs is primary + user extras only (deduped, normalized).
// Used for CLI sharing links and exports — not the merged preset catalog baked into serverNames.
func SharingLinkSNIs(primary string, extras []string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		h := NormalizeHost(s)
		if h == "" {
			return
		}
		k := strings.ToLower(h)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, h)
	}
	add(primary)
	for _, e := range extras {
		add(e)
	}
	return out
}

const MetaKey = "_tunnelbypass"

// TunnelMeta is stored in Hysteria (and optionally other) server JSON for SNI list persistence.
type TunnelMeta struct {
	ServerNames []string `json:"serverNames"`
	Version     int      `json:"version"`
}

var categoryOrder = []string{
	"gaming",
	"social",
	"streaming",
	"tech",
	"egypt",
	"general",
	"custom",
}

func CategoryOrder() []string {
	out := make([]string, len(categoryOrder))
	copy(out, categoryOrder)
	return out
}

func CategoryLabel(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "gaming":
		return "Gaming"
	case "social":
		return "Social"
	case "streaming":
		return "Streaming"
	case "tech":
		return "Tech / CDN"
	case "egypt":
		return "Egypt (EG)"
	case "general":
		return "General"
	case "custom":
		return "Custom / Other"
	default:
		return category
	}
}

func inferCategory(host string) string {
	h := NormalizeHost(host)
	if c, ok := seedHostToCategory[h]; ok {
		return c
	}
	return "custom"
}

func HostsByCategory(category string) []string {
	c := strings.ToLower(strings.TrimSpace(category))
	if c == "" || c == "all" {
		return nil
	}
	// Wizard label "Tech / CDN" maps to JSON key "cdn".
	if c == "tech" {
		c = "cdn"
	}
	var out []string
	for _, h := range DefaultHosts() {
		if c == "general" {
			if sc, ok := seedHostToCategory[h]; ok && countryCatalogCategories[sc] {
				continue
			}
			ic := inferCategory(h)
			if ic != "general" && ic != "basic" {
				continue
			}
			out = append(out, h)
			continue
		}
		if inferCategory(h) == c {
			out = append(out, h)
		}
	}
	return out
}
