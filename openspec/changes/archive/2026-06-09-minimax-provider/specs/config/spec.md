# Delta for Config (minimax-provider change)

## ADDED Requirements

### Requirement: CONFIG-MM-1 — `minimax` is a known provider type

`KnownProviders` MUST include the string `"minimax"`. `IsKnownProvider("minimax")` MUST return `true`. All provider-type validation switches (v2 active provider, v1 legacy provider, `Fallback.Type`) MUST accept `"minimax"` as a valid value without error.

(Previously: `"minimax"` was absent from `KnownProviders` and all four validation switches; using it produced a validation error.)

#### Scenario CM-1a: minimax passes IsKnownProvider

- GIVEN the global `KnownProviders` slice
- WHEN `IsKnownProvider("minimax")` is called
- THEN it returns `true`

#### Scenario CM-1b: minimax passes v2 active-provider validation

- GIVEN a config with `providers.active: minimax` and a valid `api_key`
- WHEN the config is validated
- THEN no error is returned for the provider type

#### Scenario CM-1c: minimax passes v1 legacy provider validation

- GIVEN a v1 config block with `provider.type: minimax` and a valid `api_key`
- WHEN the config is validated
- THEN no error is returned for the provider type

#### Scenario CM-1d: minimax passes Fallback.Type validation

- GIVEN a fallback config with `fallback.type: minimax` and a valid `api_key`
- WHEN the config is validated
- THEN no error is returned for the provider type

---

### Requirement: CONFIG-MM-2 — api_key is REQUIRED for minimax

A config block with `type: minimax` MUST be rejected at validation time if `api_key` is absent or empty. The standard `openai`-with-custom-base exemption (which allows `openai` to skip `api_key` when `base_url` is custom) MUST NOT apply to `minimax`.

#### Scenario CM-2a: minimax config with api_key passes validation

- GIVEN a provider config `type: minimax` with `api_key: "sk-cp-abc123"`
- WHEN the config is validated
- THEN validation succeeds

#### Scenario CM-2b: minimax config without api_key fails validation

- GIVEN a provider config `type: minimax` with no `api_key` field
- WHEN the config is validated
- THEN a non-nil validation error is returned
- AND the error message references `api_key` or the minimax provider
