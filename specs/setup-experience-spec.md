# Delta Spec: Setup Experience

## ADDED Requirements

### Requirement: Wizard Channel Selector — WhatsApp Support

The wizard MUST include "whatsapp" as a selectable channel option alongside cli, telegram, and discord.

#### Scenario: WhatsApp appears in channel list

- GIVEN the user reaches step 2 (channel selection) of the wizard
- WHEN the channel selector renders
- THEN "whatsapp" MUST appear as a choice after "discord"

#### Scenario: WhatsApp routes to channel extra step

- GIVEN the user selects "whatsapp" at step 2
- WHEN they press Enter to advance
- THEN the wizard MUST proceed to step 3 (channel extra) to collect phone_number_id and access_token

### Requirement: Wizard Store Type Defaults to File

The wizard MUST write `store.type: file` (not sqlite) and `audit.type: file` to the generated config.

#### Scenario: Generated config uses file store

- GIVEN the user completes the wizard
- WHEN the config YAML is generated and written
- THEN `store.type` MUST be `file`

#### Scenario: Audit type defaults to file

- GIVEN the wizard generates config
- WHEN the audit section is written
- THEN `audit.type` MUST be `file`

### Requirement: Broken dev.sh References Removed

Documentation and build files MUST NOT reference `./dev.sh` if the file does not exist.

#### Scenario: README has no dev.sh references

- GIVEN README.md or Makefile contain `dev.sh` references
- WHEN the cleanup is applied
- THEN those references MUST be removed or replaced with the correct invocation

### Requirement: microagent setup Subcommand

The system MUST support `microagent setup` as a subcommand that launches the interactive wizard, equivalent to `microagent --setup`.

#### Scenario: setup subcommand launches wizard

- GIVEN the user runs `microagent setup` in an interactive terminal
- WHEN the command is parsed
- THEN the setup wizard MUST launch with identical behavior to `--setup`

#### Scenario: setup subcommand rejects non-TTY

- GIVEN the user runs `microagent setup` in a non-interactive context (pipe, CI)
- WHEN the command checks for TTY
- THEN it MUST print "Setup wizard requires an interactive terminal." to stderr and exit with code 1

### Requirement: API Key Format Validation

The wizard MUST validate API key format before saving and warn (non-blocking) if the format is suspicious.

#### Scenario: Empty API key warns for non-ollama

- GIVEN the user leaves the API key field empty
- WHEN the provider is NOT ollama
- THEN a warning MUST be displayed ("API key is empty — this provider requires a key")
- AND the user MAY still proceed

#### Scenario: Valid API key accepted silently

- GIVEN the user enters a non-empty API key
- WHEN the provider is anthropic
- THEN no warning is shown

### Requirement: Config Path Respect

The wizard MUST check for an existing local `./config.yaml` before writing and offer to write there instead.

#### Scenario: Local config detected

- GIVEN a `./config.yaml` exists in the working directory
- WHEN the wizard reaches the write step
- THEN it MUST ask the user whether to overwrite the local config or write to the default location

#### Scenario: No local config uses default path

- GIVEN no `./config.yaml` exists
- WHEN the wizard writes config
- THEN it MUST write to `~/.microagent/config.yaml`

### Requirement: microagent doctor Command

The system MUST provide `microagent doctor` that validates the runtime environment.

#### Scenario: doctor validates config file

- GIVEN a config file exists at the expected path
- WHEN `microagent doctor` runs
- THEN it MUST report whether the config parses and validates successfully

#### Scenario: doctor checks required env vars

- GIVEN the config references `${SOME_VAR}` placeholders
- WHEN `microagent doctor` runs
- THEN it MUST report which environment variables are set and which are missing

#### Scenario: doctor checks store path accessibility

- GIVEN the config specifies a store.path
- WHEN `microagent doctor` runs
- THEN it MUST report whether the directory exists and is writable

## MODIFIED Requirements

_(None — all existing behavior is preserved. This is a new domain spec.)_

## REMOVED Requirements

_(None — no features are removed.)_
