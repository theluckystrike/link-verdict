# link-verdict

Go package that turns the three places a page can say "do not follow this" into one
answer: the anchor's `rel` attribute, the page's `<meta name="robots">` tags, and the
response's `X-Robots-Tag` headers.

```
go get github.com/theluckystrike/link-verdict
```

Those three are usually read by three different pieces of code that never compare
notes, so a link gets recorded as live when a header two layers away already killed
it.

## Example

```go
v := linkverdict.Evaluate(linkverdict.Input{
    Rel:        "noopener noreferrer",
    MetaRobots: []string{"index, follow"},
    XRobotsTag: []string{"googlebot: noindex"},
    UserAgent:  "googlebot",
})

v.Followable // true
v.Indexable  // false
v.String()   // followable=true indexable=false (x-robots-tag:googlebot:noindex)
```

`Verdict.Reasons` lists every applicable restriction with the source it came from,
which matters more than the booleans: a link recorded as dead should say which
directive killed it. Anchor-level `rel=nofollow` and page-level `nofollow` are kept
apart, because they fail for different operational reasons.

## Rules implemented

* `nofollow`, `ugc` and `sponsored` in `rel` each remove followability.
* `none` is equivalent to `noindex, nofollow`; `all` restricts nothing.
* A directive prefixed with a crawler name (`googlebot: noindex`) binds only that
  crawler; an unprefixed one binds every crawler.
* Where directives conflict the restrictive one wins, so `index, noindex` is noindex.
* Valued directives such as `max-snippet:-1` and `unavailable_after: ...` are not
  mistaken for a crawler prefix.
* Evaluation only ever removes permissions, so the order the sources are read in
  cannot change the answer.

No network access and no HTML parsing; you hand it strings you already have.
Standard library only.

Written for the link verification step of the pipeline at
<https://handsofflinks.com/>, where a row may not be marked live until the served
HTML has been re-read and the verdict comes back followable.

MIT licensed.
