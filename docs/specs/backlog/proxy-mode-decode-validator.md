# `ProxyMode` decode-time validation (UnmarshalYAML / UnmarshalText)

> Status: **Backlog**
> Opened: 2026-04-16
> Priority: **low**
> Est. effort: **XS** (1-2h)
> Decision required: **no**

## Problem

`type ProxyMode string` (introduced in commit `371fa73`) is
structurally open: `ProxyMode("typo")` compiles, and YAML /
mapstructure decoding writes any string into the field. The only
gate is `ChaosConfig.Validate()` which rejects non-canonical values.

If a future subcommand loads a config and skips `Validate()` — a
hypothetical `ruptor dry-run` or a library embed — invalid modes
propagate past the intended boundary.

## Proposed solution

Implement `UnmarshalYAML` and `UnmarshalText` on `*ProxyMode` so the
rejection happens at decode time, not at validation time. Keep
`Validate()` as the authoritative gate for programmatic construction
(`cfg.Proxy.Mode = ProxyMode("x")` in Go code).

```go
func (m *ProxyMode) UnmarshalYAML(value *yaml.Node) error {
    var s string
    if err := value.Decode(&s); err != nil {
        return err
    }
    return m.set(s)
}

func (m *ProxyMode) UnmarshalText(data []byte) error {
    return m.set(string(data))
}

func (m *ProxyMode) set(s string) error {
    switch s {
    case "":
        *m = ProxyModeHTTP
    case string(ProxyModeHTTP), string(ProxyModeMCP), string(ProxyModeAuto):
        *m = ProxyMode(s)
    default:
        return fmt.Errorf("proxy.mode must be one of %q, %q, %q (got %q)",
            ProxyModeHTTP, ProxyModeMCP, ProxyModeAuto, s)
    }
    return nil
}
```

Update `type ProxyMode` doc-comment to note: "Decoding from YAML /
JSON rejects non-canonical values before Validate runs; Validate
remains the gate for programmatic construction."

## Acceptance criteria

- [ ] `UnmarshalYAML`, `UnmarshalText` implemented on `*ProxyMode`.
- [ ] Unit test: YAML containing `mode: MCP`, `mode: " mcp "`,
      `mode: "bogus"` all return the same error as Validate would.
- [ ] Unit test: empty YAML `mode: ""` normalises to `ProxyModeHTTP`
      through the decode path.
- [ ] `Validate()`'s own `case ""` branch redundant (decode already
      normalised) but kept as a belt-and-suspenders for programmatic
      construction — documented as such.
- [ ] `TestChaosValidate_RejectsProxyModeCaseVariants` still passes
      (the programmatic path the test exercises is unchanged).

## Out of scope

- Splitting `Validate` into `Validate` + `Normalize` — separate spec
  (see `validate-normalize-split` if filed).

## References

- `internal/config/chaos_config.go` — `type ProxyMode`, `Validate`
- `internal/config/loader_test.go` — existing test matrix
- Previous DE review flag: review round 3 "Concerns"
