package linkverdict

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseRel(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"noopener", []string{"noopener"}},
		{"  NOOPENER   NoReferrer ", []string{"noopener", "noreferrer"}},
		{"nofollow,ugc", []string{"nofollow", "ugc"}},
		{"nofollow nofollow", []string{"nofollow"}},
		{"noopener\tnoreferrer\nnofollow", []string{"noopener", "noreferrer", "nofollow"}},
	}
	for _, c := range cases {
		got := ParseRel(c.in)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Fatalf("ParseRel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRelBlocksFollow(t *testing.T) {
	cases := []struct {
		in      string
		blocked bool
		tokens  string
	}{
		{"", false, "[]"},
		{"noopener noreferrer", false, "[]"},
		{"nofollow", true, "[nofollow]"},
		{"UGC", true, "[ugc]"},
		{"sponsored noopener", true, "[sponsored]"},
		{"nofollow ugc sponsored", true, "[nofollow sponsored ugc]"},
	}
	for _, c := range cases {
		blocked, tokens := RelBlocksFollow(c.in)
		if blocked != c.blocked {
			t.Fatalf("RelBlocksFollow(%q) blocked = %v, want %v", c.in, blocked, c.blocked)
		}
		if got := fmt.Sprint(tokens); got != c.tokens && !(len(tokens) == 0 && c.tokens == "[]") {
			t.Fatalf("RelBlocksFollow(%q) tokens = %v, want %v", c.in, got, c.tokens)
		}
	}
}

func TestParseDirectives(t *testing.T) {
	ds := ParseDirectives("index, follow")
	if len(ds) != 2 || ds[0].Value != "index" || ds[1].Value != "follow" {
		t.Fatalf("got %+v", ds)
	}
	if ds[0].UserAgent != "" {
		t.Fatalf("unprefixed directive should have no user agent, got %q", ds[0].UserAgent)
	}
}

func TestParseDirectivesUserAgentPrefix(t *testing.T) {
	ds := ParseDirectives("Googlebot: noindex, nofollow")
	if len(ds) != 2 {
		t.Fatalf("got %d directives, want 2: %+v", len(ds), ds)
	}
	for _, d := range ds {
		if d.UserAgent != "googlebot" {
			t.Fatalf("user agent = %q, want googlebot", d.UserAgent)
		}
	}
	if ds[0].Value != "noindex" || ds[1].Value != "nofollow" {
		t.Fatalf("values = %q, %q", ds[0].Value, ds[1].Value)
	}
}

func TestParseDirectivesKeepsValuedDirectivesIntact(t *testing.T) {
	ds := ParseDirectives("max-snippet:-1")
	if len(ds) != 1 {
		t.Fatalf("got %+v", ds)
	}
	if ds[0].UserAgent != "" {
		t.Fatalf("max-snippet must not be read as a crawler name, got %q", ds[0].UserAgent)
	}
	if ds[0].Value != "max-snippet:-1" {
		t.Fatalf("value = %q", ds[0].Value)
	}

	ds = ParseDirectives("unavailable_after: 2026-12-31T23:59:59Z")
	if len(ds) != 1 || ds[0].UserAgent != "" {
		t.Fatalf("unavailable_after must not be read as a crawler name, got %+v", ds)
	}
}

func TestParseDirectivesEmpty(t *testing.T) {
	if ds := ParseDirectives("   "); len(ds) != 0 {
		t.Fatalf("got %+v", ds)
	}
	if ds := ParseDirectives("index, , follow"); len(ds) != 2 {
		t.Fatalf("empty fragments should be dropped, got %+v", ds)
	}
}

func TestDirectiveAppliesTo(t *testing.T) {
	all := Directive{Value: "noindex"}
	if !all.AppliesTo("googlebot") || !all.AppliesTo("") {
		t.Fatal("an unprefixed directive applies to every crawler")
	}
	named := Directive{UserAgent: "googlebot", Value: "noindex"}
	if !named.AppliesTo("GoogleBot") {
		t.Fatal("crawler matching should be case-insensitive")
	}
	if named.AppliesTo("bingbot") || named.AppliesTo("") {
		t.Fatal("a named directive should not apply to another crawler")
	}
}

func TestEvaluateCleanLink(t *testing.T) {
	v := Evaluate(Input{Rel: "noopener noreferrer", MetaRobots: []string{"index, follow"}})
	if !v.Followable || !v.Indexable || v.Blocked() {
		t.Fatalf("clean link should pass: %s", v)
	}
	if len(v.Reasons) != 0 {
		t.Fatalf("no reasons expected: %+v", v.Reasons)
	}
}

func TestEvaluateAbsentRelIsFollowable(t *testing.T) {
	v := Evaluate(Input{})
	if !v.Followable || !v.Indexable {
		t.Fatalf("an absent rel and no robots directives should pass: %s", v)
	}
}

func TestEvaluateRelNofollow(t *testing.T) {
	v := Evaluate(Input{Rel: "nofollow noopener"})
	if v.Followable {
		t.Fatal("rel=nofollow should not be followable")
	}
	if !v.Indexable {
		t.Fatal("rel=nofollow says nothing about indexability of the page")
	}
	if len(v.Reasons) != 1 || v.Reasons[0].Source != SourceRel || v.Reasons[0].Directive != "nofollow" {
		t.Fatalf("reasons = %+v", v.Reasons)
	}
}

func TestEvaluateUGCAndSponsored(t *testing.T) {
	for _, rel := range []string{"ugc", "sponsored", "UGC noopener"} {
		if v := Evaluate(Input{Rel: rel}); v.Followable {
			t.Fatalf("rel=%q should not be followable", rel)
		}
	}
}

func TestEvaluateMetaNone(t *testing.T) {
	v := Evaluate(Input{MetaRobots: []string{"none"}})
	if v.Followable || v.Indexable {
		t.Fatalf("none means noindex, nofollow: %s", v)
	}
	if len(v.Reasons) != 1 || v.Reasons[0].Directive != "none" {
		t.Fatalf("reasons = %+v", v.Reasons)
	}
}

func TestEvaluateMetaAllIsPermissive(t *testing.T) {
	v := Evaluate(Input{MetaRobots: []string{"all"}})
	if !v.Followable || !v.Indexable {
		t.Fatalf("all should not restrict anything: %s", v)
	}
}

func TestEvaluateRestrictiveWins(t *testing.T) {
	v := Evaluate(Input{MetaRobots: []string{"index, noindex"}})
	if v.Indexable {
		t.Fatalf("conflicting directives should resolve to the restrictive one: %s", v)
	}
}

func TestEvaluateXRobotsTagHeader(t *testing.T) {
	v := Evaluate(Input{
		Rel:        "noopener",
		XRobotsTag: []string{"noindex, nofollow"},
	})
	if v.Followable || v.Indexable {
		t.Fatalf("header directives should apply: %s", v)
	}
	for _, r := range v.Reasons {
		if r.Source != SourceXRobotsTag {
			t.Fatalf("wrong source: %+v", r)
		}
	}
}

func TestEvaluateCrawlerSpecificHeader(t *testing.T) {
	in := Input{
		Rel:        "noopener noreferrer",
		MetaRobots: []string{"index, follow"},
		XRobotsTag: []string{"googlebot: noindex"},
		UserAgent:  "googlebot",
	}
	v := Evaluate(in)
	if !v.Followable {
		t.Fatalf("nothing here removes follow: %s", v)
	}
	if v.Indexable {
		t.Fatalf("googlebot-specific noindex should apply to googlebot: %s", v)
	}

	in.UserAgent = "bingbot"
	v = Evaluate(in)
	if !v.Indexable {
		t.Fatalf("a googlebot-specific directive should not bind bingbot: %s", v)
	}

	in.UserAgent = ""
	v = Evaluate(in)
	if !v.Indexable {
		t.Fatalf("a named directive should not bind a generic crawler: %s", v)
	}
}

func TestEvaluateRepeatedHeaders(t *testing.T) {
	v := Evaluate(Input{XRobotsTag: []string{"noarchive", "noindex"}})
	if v.Indexable {
		t.Fatalf("a second X-Robots-Tag header should still count: %s", v)
	}
	if len(v.Reasons) != 1 {
		t.Fatalf("noarchive is not a follow or index restriction, reasons = %+v", v.Reasons)
	}
}

func TestEvaluateSourceOrderAndString(t *testing.T) {
	v := Evaluate(Input{
		Rel:        "nofollow",
		MetaRobots: []string{"noindex"},
		XRobotsTag: []string{"nofollow"},
	})
	if len(v.Reasons) != 3 {
		t.Fatalf("reasons = %+v", v.Reasons)
	}
	wantOrder := []string{SourceRel, SourceMetaRobots, SourceXRobotsTag}
	for i, w := range wantOrder {
		if v.Reasons[i].Source != w {
			t.Fatalf("reason %d source = %q, want %q", i, v.Reasons[i].Source, w)
		}
	}
	s := v.String()
	if !strings.Contains(s, "followable=false") || !strings.Contains(s, "rel:nofollow") {
		t.Fatalf("String() = %q", s)
	}
}

func TestEvaluateIsOrderIndependent(t *testing.T) {
	a := Evaluate(Input{MetaRobots: []string{"noindex", "follow"}})
	b := Evaluate(Input{MetaRobots: []string{"follow", "noindex"}})
	if a.Followable != b.Followable || a.Indexable != b.Indexable {
		t.Fatalf("order changed the verdict: %s vs %s", a, b)
	}
}

func TestVerdictBlocked(t *testing.T) {
	if (Verdict{Followable: true, Indexable: true}).Blocked() {
		t.Fatal("a clean verdict is not blocked")
	}
	if !(Verdict{Followable: false, Indexable: true}).Blocked() {
		t.Fatal("a nofollow verdict is blocked")
	}
}
