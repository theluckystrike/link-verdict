// Package linkverdict turns the three places a page can say "do not follow
// this" into one answer. A link is only worth counting when the anchor's rel
// attribute, the page's <meta name="robots"> tags and the response's
// X-Robots-Tag headers all permit it, and those three are usually read by
// three different pieces of code that never compare notes.
//
// Evaluate reads all three, applies the most restrictive directive that
// applies to the crawler you name, and returns a Verdict carrying Followable,
// Indexable and the list of reasons that decided it. The reasons matter more
// than the booleans: a link recorded as dead should say which directive
// killed it, and from which of the three sources.
//
//	v := linkverdict.Evaluate(linkverdict.Input{
//		Rel:        "noopener noreferrer",
//		MetaRobots: []string{"index, follow"},
//		XRobotsTag: []string{"googlebot: noindex"},
//		UserAgent:  "googlebot",
//	})
//	// v.Followable == true, v.Indexable == false,
//	// v.Reasons contains the X-Robots-Tag directive that did it.
//
// Rules the package implements, all of them from the robots meta and
// X-Robots-Tag specification: "none" is equivalent to "noindex, nofollow";
// "all" is equivalent to "index, follow"; a directive prefixed with a crawler
// name applies only to that crawler; an unprefixed directive applies to every
// crawler; and where directives conflict the restrictive one wins, so
// "index, noindex" is noindex.
//
// A page-level nofollow and an anchor-level rel=nofollow are kept apart in
// the reasons, because they fail for different operational causes: one is the
// host's policy for the whole page, the other is what the publishing form did
// to your anchor.
//
// No network access and no HTML parsing; you hand it strings you already
// have. Standard library only. Written for the link verification step of the
// autonomous link pipeline at https://handsofflinks.com/ where a row may not
// be marked live until the served HTML has been re-read and this verdict
// comes back followable.
package linkverdict

import (
	"sort"
	"strings"
)

// Rel tokens that remove a link's ability to pass ranking signals.
const (
	RelNofollow  = "nofollow"
	RelUGC       = "ugc"
	RelSponsored = "sponsored"
)

// Source names where a directive came from, as used in Reason.Source.
const (
	SourceRel        = "rel"
	SourceMetaRobots = "meta-robots"
	SourceXRobotsTag = "x-robots-tag"
)

// ParseRel splits a rel attribute into lower-cased tokens, in the order they
// appeared, without duplicates. Tokens are separated by any run of ASCII
// whitespace, and a comma-separated attribute is accepted too because
// publishing forms emit it often enough to be worth tolerating.
func ParseRel(rel string) []string {
	fields := strings.FieldsFunc(rel, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == ','
	})
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		t := strings.ToLower(f)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// RelBlocksFollow reports whether a rel attribute contains a token that stops
// the link passing ranking signals, and returns the tokens responsible,
// sorted so the result is stable.
func RelBlocksFollow(rel string) (bool, []string) {
	blocking := map[string]bool{RelNofollow: true, RelUGC: true, RelSponsored: true}
	var found []string
	for _, t := range ParseRel(rel) {
		if blocking[t] {
			found = append(found, t)
		}
	}
	sort.Strings(found)
	return len(found) > 0, found
}

// Directive is one robots instruction: an optional crawler name, and the
// directive itself lower-cased.
type Directive struct {
	// UserAgent is the crawler the directive was addressed to, lower-cased,
	// or "" when the directive was unprefixed and so applies to all.
	UserAgent string
	// Value is the directive, lower-cased, for example "noindex".
	Value string
	// Raw is the fragment as it appeared, trimmed.
	Raw string
}

// AppliesTo reports whether d binds the named crawler. An unprefixed
// directive binds every crawler. Matching is case-insensitive. An empty
// userAgent means "evaluate for a generic crawler", under which only
// unprefixed directives apply.
func (d Directive) AppliesTo(userAgent string) bool {
	if d.UserAgent == "" {
		return true
	}
	return strings.EqualFold(d.UserAgent, userAgent)
}

// ParseDirectives parses one robots directive string, as found in the content
// attribute of a robots meta tag or in the value of an X-Robots-Tag header.
// It handles a leading crawler name ("googlebot: noindex, nofollow"), which
// then applies to every directive in that string.
//
// Directives that take a value, such as "max-snippet:-1", keep the whole
// fragment as Value; only a leading crawler name is split off, and it is
// recognised as a crawler name only when what follows it contains a known
// bare directive or another comma-separated fragment.
func ParseDirectives(s string) []Directive {
	agent := ""
	body := strings.TrimSpace(s)

	if i := strings.Index(body, ":"); i > 0 {
		head := strings.TrimSpace(body[:i])
		tail := strings.TrimSpace(body[i+1:])
		if !strings.ContainsAny(head, " ,") && !isValuedDirective(head) && tail != "" {
			agent = strings.ToLower(head)
			body = tail
		}
	}

	var out []Directive
	for _, part := range strings.Split(body, ",") {
		raw := strings.TrimSpace(part)
		if raw == "" {
			continue
		}
		out = append(out, Directive{
			UserAgent: agent,
			Value:     strings.ToLower(raw),
			Raw:       raw,
		})
	}
	return out
}

// valuedDirectives are the directives that legitimately carry a colon, so
// their name must never be mistaken for a crawler name.
var valuedDirectives = map[string]bool{
	"max-snippet":          true,
	"max-image-preview":    true,
	"max-video-preview":    true,
	"unavailable_after":    true,
	"notranslate":          false,
	"noimageindex":         false,
	"nositelinkssearchbox": false,
}

func isValuedDirective(name string) bool {
	return valuedDirectives[strings.ToLower(name)]
}

// Reason records one directive that shaped the verdict.
type Reason struct {
	// Source is SourceRel, SourceMetaRobots or SourceXRobotsTag.
	Source string
	// Directive is the token that applied, for example "nofollow".
	Directive string
	// UserAgent is the crawler the directive named, or "" if unprefixed.
	UserAgent string
}

// Input is everything read off one fetched page for one anchor.
type Input struct {
	// Rel is the rel attribute of the anchor, exactly as served. Empty means
	// the attribute was absent, which is followable.
	Rel string
	// MetaRobots holds the content attribute of every <meta name="robots">
	// or crawler-specific robots meta on the page.
	MetaRobots []string
	// XRobotsTag holds the value of every X-Robots-Tag response header. A
	// header sent more than once contributes one entry per occurrence.
	XRobotsTag []string
	// UserAgent is the crawler being evaluated for, for example "googlebot".
	// Empty evaluates for a generic crawler, so crawler-specific directives
	// are ignored.
	UserAgent string
}

// Verdict is the combined answer.
type Verdict struct {
	// Followable is false when any applicable directive removes the link's
	// ability to pass ranking signals.
	Followable bool
	// Indexable is false when any applicable directive keeps the page
	// carrying the link out of the index.
	Indexable bool
	// Reasons lists every applicable restriction found, in source order:
	// rel first, then meta robots, then X-Robots-Tag.
	Reasons []Reason
}

// Blocked reports whether the link is not both followable and indexable.
func (v Verdict) Blocked() bool { return !v.Followable || !v.Indexable }

// String renders the verdict compactly, for example
// "followable=false indexable=true (rel:nofollow)".
func (v Verdict) String() string {
	var b strings.Builder
	b.WriteString("followable=")
	b.WriteString(boolString(v.Followable))
	b.WriteString(" indexable=")
	b.WriteString(boolString(v.Indexable))
	if len(v.Reasons) == 0 {
		return b.String()
	}
	parts := make([]string, 0, len(v.Reasons))
	for _, r := range v.Reasons {
		if r.UserAgent != "" {
			parts = append(parts, r.Source+":"+r.UserAgent+":"+r.Directive)
			continue
		}
		parts = append(parts, r.Source+":"+r.Directive)
	}
	b.WriteString(" (")
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(")")
	return b.String()
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Evaluate combines the anchor's rel attribute, the page's robots meta tags
// and the response's X-Robots-Tag headers into one verdict for the crawler
// named in in.UserAgent. It starts from followable and indexable and only
// ever removes permissions, so the order the sources are read in cannot
// change the answer.
func Evaluate(in Input) Verdict {
	v := Verdict{Followable: true, Indexable: true}

	if blocked, tokens := RelBlocksFollow(in.Rel); blocked {
		v.Followable = false
		for _, t := range tokens {
			v.Reasons = append(v.Reasons, Reason{Source: SourceRel, Directive: t})
		}
	}

	apply := func(source string, values []string) {
		for _, raw := range values {
			for _, d := range ParseDirectives(raw) {
				if !d.AppliesTo(in.UserAgent) {
					continue
				}
				switch d.Value {
				case "none":
					v.Followable = false
					v.Indexable = false
				case "noindex":
					v.Indexable = false
				case "nofollow":
					v.Followable = false
				default:
					continue
				}
				v.Reasons = append(v.Reasons, Reason{
					Source:    source,
					Directive: d.Value,
					UserAgent: d.UserAgent,
				})
			}
		}
	}

	apply(SourceMetaRobots, in.MetaRobots)
	apply(SourceXRobotsTag, in.XRobotsTag)
	return v
}
