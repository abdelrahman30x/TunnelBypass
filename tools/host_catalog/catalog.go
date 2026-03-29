package host_catalog

import (
	_ "embed"
	"encoding/json"
	"math/rand"
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

	realityDestHosts = uniqueHosts(f.RealityDest)

	seedHostToCategory = map[string]string{}
	var allHosts []string

	seenCat := map[string]bool{}
	addHostsForCategory := func(cat string, hosts []string) {
		for _, h := range hosts {
			nh := normalizeHost(h)
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

func normalizeHost(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.Index(h, "/"); i >= 0 {
		h = h[:i]
	}
	return h
}

func uniqueHosts(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range in {
		n := normalizeHost(h)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
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
	h := normalizeHost(host)
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
	h := normalizeHost(host)
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

// RandomRealityDestHost picks a host suitable for Reality TCP dest.
func RandomRealityDestHost() string {
	h := realityDestHosts
	if len(h) == 0 {
		return RandomHost()
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return h[r.Intn(len(h))]
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

// ServerNamesForVLESS builds Reality serverNames: primary + extras, or full catalog if empty.
func ServerNamesForVLESS(primary string, extras []string) []string {
	if primary == "" && len(extras) == 0 {
		return DefaultHosts()
	}
	return SNIsForSharing(primary, extras)
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
	h := normalizeHost(host)
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
	var out []string
	for _, h := range DefaultHosts() {
		if c == "general" {
			if sc, ok := seedHostToCategory[h]; ok && countryCatalogCategories[sc] {
				continue
			}
			if inferCategory(h) != "general" {
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
