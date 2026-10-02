package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The keys of the config file that hold layers rather than settings.
const (
	hostsKey    = "hosts"
	profilesKey = "profiles"
	accountsKey = "accounts"
)

// File is the config file read over the defaults, in its layers: the top
// level, the settings for each host under hosts, and those for each
// profile under profiles, which applies to the accounts it lists. Resolve
// lays the ones that apply to a session over each other. Every
// combination that can apply was validated when the file was loaded, so
// a bad value for another host is reported on this one too.
type File struct {
	// base is the top level of the file, without hosts and profiles,
	// with no settings for a missing or empty file.
	base layer
	// hosts are the layers of hosts, by host as gh names it, in
	// lowercase.
	hosts map[string]layer
	// profiles are the layers of profiles, in the order of the file.
	profiles []profile
	// cfg is the top level resolved, which is what the session uses
	// before it knows its host, such as for the log.
	cfg Config
	// path is where the file is, and exists whether there was one.
	path   string
	exists bool
}

// layer is a group of settings of the file, at the top level or under a
// host or a profile.
type layer struct {
	// node holds the settings under their new names, or is nil.
	node *yaml.Node
	// renamed are the settings it has under old names, named by their
	// paths in the file.
	renamed []Renamed
}

// profile is a layer of the file that applies to the accounts it lists.
type profile struct {
	layer
	name string
	// accounts are the accounts it is for, as login@host in lowercase,
	// since GitHub matches logins without regard to case.
	accounts []string
}

// Source says which layers of the file a config was resolved from, for
// the user to see why a setting has its value.
type Source struct {
	// Host is the host of the session, and HostLayer whether the file has
	// settings for it under hosts.
	Host      string
	HostLayer bool
	// Account is the account of the session, as login@host, or "" when
	// the token names none.
	Account string
	// Profile is the name of the profile that lists Account, or "".
	Profile string

	// f is the file the config was resolved from, or nil, and applied
	// the layers of it that applied, in the order they were laid on.
	// :config finds the line of each value in them only when it opens,
	// so that startup doesn't walk them.
	f       *File
	applied []applied
	// file is the config the layers made, before $GH_TUI_LOG.
	file Config
}

// applied is a layer of the file that applied to a session, and its name:
// "" for the top level, else "hosts.<host>" or "profiles.<name>".
type applied struct {
	name string
	layer
}

// Origin is where the config file sets a value: the layer it is in, as
// applied names it, and its line in the file.
type Origin struct {
	Layer string
	Line  int
}

// Path returns the path of the config file, whether or not it exists, or
// "" for a Source that no file resolved.
func (s Source) Path() string {
	if s.f == nil {
		return ""
	}
	return s.f.path
}

// Exists reports whether there was a config file.
func (s Source) Exists() bool { return s.f != nil && s.f.exists }

// Renamed returns the settings that the layers that applied have under
// the names they had before they were renamed.
func (s Source) Renamed() []Renamed {
	var out []Renamed
	for _, a := range s.applied {
		out = append(out, a.renamed...)
	}
	return out
}

// origins returns where the file sets each value of the session, by its
// path, such as "ui.icons": a later layer's line over an earlier one's.
func (s Source) origins() map[string]Origin {
	out := map[string]Origin{}
	for _, a := range s.applied {
		if a.node != nil {
			fileLines(a.node, "", a.name, out, a.renamed)
		}
	}
	return out
}

// Header returns the lines that head the config of a session where it
// is shown, such as by :config: the file at path it was read from, and
// the host, the account and the profile the session resolved it for, or
// why no profile applies.
func (s Source) Header(path string) []string {
	host := s.Host + " (the file has no settings under hosts." + s.Host + ")"
	if s.HostLayer {
		host = s.Host + " (hosts." + s.Host + " applies)"
	}
	account, profile := s.Account, s.Profile
	switch {
	case account == "":
		account, profile = "none: the token names no account", "none: no account to pick one by"
	case profile == "":
		profile = "none lists " + account
	}
	return []string{"file: " + path, "host: " + host, "account: " + account, "profile: " + profile}
}

// Renamed returns the settings that the file has under the names they
// had before they were renamed, which it was read with under their new
// names, for the user to be told.
func (f *File) Renamed() []Renamed {
	out := slices.Clone(f.base.renamed)
	for _, host := range slices.Sorted(maps.Keys(f.hosts)) {
		out = append(out, f.hosts[host].renamed...)
	}
	for _, p := range f.profiles {
		out = append(out, p.renamed...)
	}
	return out
}

// Base returns the config of the top level of the file, over the
// defaults, without the settings of any host or profile: what applies
// before the session knows its host, such as the log's settings, which
// are the same for every host.
func (f *File) Base() Config { return f.cfg.clone() }

// Resolve returns the config of a session with host, as gh names it, and
// login, the account gh stores its token for, or "" when the token names
// none: the defaults, then the top level of the file, then its settings
// for host, then those of the profile that lists login@host, with
// $GH_TUI_LOG over the log level. It says which layers applied.
func (f *File) Resolve(host, login string) (Config, Source, error) {
	host = normalizeHost(host)
	src := Source{Host: host}
	_, src.HostLayer = f.hosts[host]
	if login != "" {
		src.Account = strings.ToLower(login) + "@" + host
	}
	var p *profile
	if src.Account != "" {
		if i := slices.IndexFunc(f.profiles, func(p profile) bool { return slices.Contains(p.accounts, src.Account) }); i >= 0 {
			p = &f.profiles[i]
			src.Profile = p.name
		}
	}
	src.f, src.applied = f, f.layers(host, p)
	cfg, err := f.resolve(host, p, &src.file)
	return cfg, src, err
}

// layers returns the layers of f that apply with host and p, if not nil,
// in the order they are laid on.
func (f *File) layers(host string, p *profile) []applied {
	out := []applied{{name: "", layer: f.base}}
	if l, ok := f.hosts[host]; ok {
		out = append(out, applied{name: join(hostsKey, host), layer: l})
	}
	if p != nil {
		out = append(out, applied{name: join(profilesKey, p.name), layer: p.layer})
	}
	return out
}

// resolve returns the config of the defaults, the top level, the layer
// of host, if any, and p, if not nil, validated. If before isn't nil, it
// is set to the config before $GH_TUI_LOG, as the layers made it.
func (f *File) resolve(host string, p *profile, before *Config) (Config, error) {
	tree := defaultTree()
	var (
		renamed []Renamed
		nodes   []*yaml.Node
	)
	for _, l := range []layer{f.base, f.hosts[host], p.settings()} {
		if l.node == nil {
			continue
		}
		tree = merge(tree, l.node)
		nodes = append(nodes, l.node)
		renamed = append(shadowed(renamed, l.node), l.renamed...)
	}
	// A value of the wrong type is reported at its line in the file, which
	// the nodes laid over the defaults keep, one line for each, as the
	// other problems are.
	cfg, err := decode(tree)
	if te, ok := errors.AsType[*yaml.TypeError](err); ok {
		return Config{}, errors.New(strings.Join(te.Errors, "\n"))
	}
	if err != nil {
		return Config{}, err
	}
	if before != nil {
		*before = cfg.clone()
	}
	if level := os.Getenv(EnvLog); level != "" {
		cfg.Log.Level = strings.ToLower(level)
	}
	return cfg, atLines(renamedErrors(cfg.Validate(), renamed), nodes, renamed)
}

// atLines returns err, an error of Validate, with each problem that
// names a setting the file sets prefixed with the line of the value in
// effect: that of the last of nodes, the layers laid over each other, to
// set it. A value that a rename made has no line of its own, so it is
// given the line of the old name it was made from, among renamed, or none.
func atLines(err error, nodes []*yaml.Node, renamed []Renamed) error {
	if err == nil {
		return nil
	}
	lines := strings.Split(err.Error(), "\n")
	for i, line := range lines {
		p := problemPath(line)
		if p == "" {
			continue
		}
		for _, n := range slices.Backward(nodes) {
			if _, v := find(n, p); v != nil {
				if at := lineOf(v, p, renamed); at > 0 {
					lines[i] = fmt.Sprintf("line %d: %s", at, line)
				}
				break
			}
		}
	}
	return errors.New(strings.Join(lines, "\n"))
}

// lineOf returns the line of v, the value of the setting p, or if it has
// none, that of the last old name among renamed it was made from, or 0.
func lineOf(v *yaml.Node, p string, renamed []Renamed) int {
	if v.Line > 0 {
		return v.Line
	}
	for _, r := range slices.Backward(renamed) {
		if slices.Contains(r.New, p) {
			return r.Line
		}
	}
	return 0
}

// problemPath returns the setting a line of an error of Validate is
// about, such as "sync.interval" for "sync.interval: must be…" and for
// "sync.every (now sync.interval): must be…", or "" for a line that
// names none, or has its line already.
func problemPath(line string) string {
	head, _, ok := strings.Cut(line, ":")
	if !ok || strings.HasPrefix(line, "line ") {
		return ""
	}
	if _, now, ok := strings.Cut(head, " (now "); ok {
		return strings.TrimSuffix(now, ")")
	}
	head, _, _ = strings.Cut(head, "[")
	if strings.ContainsAny(head, " ,") {
		return ""
	}
	return head
}

// shadowed returns renamed, the old names of the layers below n, without
// the new names that n sets, whose values are then n's: an error about
// one of them is about n's value, not theirs.
func shadowed(renamed []Renamed, n *yaml.Node) []Renamed {
	var out []Renamed
	for _, r := range renamed {
		r.New = slices.DeleteFunc(slices.Clone(r.New), func(p string) bool {
			_, v := find(n, p)
			return v != nil
		})
		if len(r.New) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// settings returns the layer of p, or none for no profile.
func (p *profile) settings() layer {
	if p == nil {
		return layer{}
	}
	return p.layer
}

// parseFile reads the user's file, data, into its layers, each with its
// renamed settings moved to their new names, and every key of each known
// and allowed where it is, so that a typo, or a setting that can't differ
// between hosts, is reported even under a host the session isn't on.
func parseFile(data []byte) (*File, error) {
	f := &File{hosts: map[string]layer{}}
	root, err := parseYAML(data)
	if err != nil || root == nil {
		return f, err
	}
	root, err = plain(root, "")
	if err != nil {
		return nil, err
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: the file must hold settings, as key: value", root.Line)
	}
	hosts, profiles := take(root, hostsKey), take(root, profilesKey)
	var errs []error
	if f.base, err = parseLayer(root, ""); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, f.parseHosts(hosts), f.parseProfiles(profiles))
	return f, errors.Join(errs...)
}

// parseHosts reads n, the value of hosts, into the layers of f.
func (f *File) parseHosts(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: hosts must hold a group of settings for each host, as host: {…}", n.Line)
	}
	var errs []error
	lines := map[string]int{}
	for i := 0; i < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		host := normalizeHost(k.Value)
		if line, ok := lines[host]; ok {
			errs = append(errs, fmt.Errorf("line %d: hosts.%s is the host of line %d too", k.Line, k.Value, line))
			continue
		}
		lines[host] = k.Line
		l, err := parseLayer(v, join(hostsKey, k.Value))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f.hosts[host] = l
	}
	return errors.Join(errs...)
}

// parseProfiles reads n, the value of profiles, into the layers of f.
func (f *File) parseProfiles(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: profiles must hold a group of settings for each profile, as name: {accounts: […], …}", n.Line)
	}
	var errs []error
	owner := map[string]string{}
	for i := 0; i < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		path := join(profilesKey, k.Value)
		if v.Kind != yaml.MappingNode {
			errs = append(errs, fmt.Errorf("line %d: %s must hold its accounts and settings, as {accounts: […], …}", v.Line, path))
			continue
		}
		v = clone(v)
		accounts, lines, err := parseAccounts(take(v, accountsKey), v, path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for i, a := range accounts {
			if other, ok := owner[a]; ok {
				errs = append(errs, fmt.Errorf("line %d: %s lists %s, which profile %s lists too: an account has one profile", lines[i], path, a, other))
			}
			owner[a] = k.Value
		}
		l, err := parseLayer(v, path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f.profiles = append(f.profiles, profile{layer: l, name: k.Value, accounts: accounts})
	}
	return errors.Join(errs...)
}

// parseAccounts returns the accounts that n, the accounts of the profile
// at path, lists, as login@host in lowercase, and the line of each. p is
// the profile, for the line of a missing list.
func parseAccounts(n, p *yaml.Node, path string) (accounts []string, lines []int, err error) {
	want := fmt.Errorf("%s.%s must list the accounts the profile is for, as [login@host, …]", path, accountsKey)
	if n == nil {
		return nil, nil, fmt.Errorf("line %d: %w", p.Line, want)
	}
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		return nil, nil, fmt.Errorf("line %d: %w", n.Line, want)
	}
	var errs []error
	for _, item := range n.Content {
		login, host, ok := strings.Cut(item.Value, "@")
		if item.Kind != yaml.ScalarNode || !ok || login == "" || normalizeHost(host) == "" || strings.ContainsAny(login, "/ \t") || strings.ContainsAny(host, "@/ \t") {
			errs = append(errs, fmt.Errorf("line %d: %s: %q isn't an account: write it as login@host, such as octocat@github.com", item.Line, path, item.Value))
			continue
		}
		a := strings.ToLower(login) + "@" + normalizeHost(host)
		if slices.Contains(accounts, a) {
			errs = append(errs, fmt.Errorf("line %d: %s lists %s twice", item.Line, path, item.Value))
			continue
		}
		accounts, lines = append(accounts, a), append(lines, item.Line)
	}
	return accounts, lines, errors.Join(errs...)
}

// parseLayer returns the layer of n, the settings at path, "" for the
// top level, with its renamed settings moved to their new names, once it
// is sure every key of it is known and, below the top level, allowed
// there.
func parseLayer(n *yaml.Node, path string) (layer, error) {
	if n.Kind != yaml.MappingNode {
		return layer{}, fmt.Errorf("line %d: %s must hold settings, as key: value", n.Line, name(path))
	}
	if path != "" {
		n = clone(n)
	}
	// Below the top level, a global setting is reported by the name the
	// file has, its old one too, so it is looked for before the renamed
	// settings move.
	var errs []error
	if path != "" {
		errs = checkGlobal(n, "")
	}
	renamed, err := migrate(n, renames, path == "")
	if err != nil {
		return layer{}, within(path, errors.Join(append(errs, err)...))
	}
	for i, r := range renamed {
		if path != "" && !Global(r.Old) {
			for _, p := range r.New {
				if Global(p) {
					errs = append(errs, fmt.Errorf("line %d: %s (now %s) can only be set at the top level of the file, since it is the same for every host and account", r.Line, r.Old, p))
				}
			}
		}
		renamed[i].Old = join(path, r.Old)
	}
	errs = append(errs, checkKnown(n, reflect.TypeFor[Config](), "")...)
	if err := errors.Join(errs...); err != nil {
		return layer{}, within(path, err)
	}
	return layer{node: n, renamed: renamed}, nil
}

// checkGlobal returns an error for each setting of the tree n, a layer
// below the top level, at path, that is global (see [Global]): one about
// the machine, the terminal or the person rather than the host, such as
// the keys, or one read before the host is known, such as the log's.
func checkGlobal(n *yaml.Node, path string) []error {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	var errs []error
	for i := 0; i < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		p := join(path, k.Value)
		if Global(p) {
			errs = append(errs, fmt.Errorf("line %d: %s can only be set at the top level of the file, since it is the same for every host and account", k.Line, p))
			continue
		}
		errs = append(errs, checkGlobal(v, p)...)
	}
	return errs
}

// validate checks every combination of the layers of f that a session
// can resolve: the top level, over which each host, and over which each
// profile for each of its accounts. It reports each problem
// once, where it first appears, and keeps the top level's config.
func (f *File) validate() error {
	base, err := f.resolve("", nil, nil)
	if err != nil {
		return err
	}
	f.cfg = base
	var errs []error
	bad := map[string]bool{}
	for _, host := range slices.Sorted(maps.Keys(f.hosts)) {
		if _, err := f.resolve(host, nil, nil); err != nil {
			bad[host] = true
			errs = append(errs, fmt.Errorf("with hosts.%s:\n%w", host, err))
		}
	}
	for i := range f.profiles {
		p := &f.profiles[i]
		// What is wrong with the profile itself is wrong for each of its
		// accounts, so it is told at the first, and a later account adds
		// only what its host's settings bring. The profile isn't checked
		// on its own, since a value may be valid only with a host's.
		seen := map[string]bool{}
		for _, a := range p.accounts {
			_, host, _ := strings.Cut(a, "@")
			if bad[host] {
				continue
			}
			if _, err := f.resolve(host, p, nil); err != nil {
				if err := unseen(err, seen); err != nil {
					errs = append(errs, fmt.Errorf("with profile %s for %s:\n%w", p.name, a, err))
				}
			}
		}
	}
	return errors.Join(errs...)
}

// unseen returns the lines of err that seen doesn't hold, and adds them
// to it, or nil when it holds them all.
func unseen(err error, seen map[string]bool) error {
	var out []string
	for line := range strings.SplitSeq(err.Error(), "\n") {
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return errors.New(strings.Join(out, "\n"))
}

// take removes key from the mapping m, and returns its value, or nil.
func take(m *yaml.Node, key string) *yaml.Node {
	i := mappingIndex(m, key)
	if i < 0 {
		return nil
	}
	v := m.Content[i+1]
	m.Content = append(m.Content[:i:i], m.Content[i+2:]...)
	return v
}

// clone returns a copy of the mapping m whose content the caller may
// change, sharing the nodes of its keys and values.
func clone(m *yaml.Node) *yaml.Node {
	out := *m
	out.Content = slices.Clone(m.Content)
	return &out
}

// normalizeHost returns host as gh names it: in lowercase, without a
// trailing dot.
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// within returns err, of the layer at path, prefixed with the layer
// unless it is the top level.
func within(path string, err error) error {
	if path == "" {
		return err
	}
	return fmt.Errorf("%s: %w", path, err)
}
