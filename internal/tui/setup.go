package tui

// setup.go — embedded first-run setup TUI (PR-B1).
//
// RunSetupTUI launches a self-contained bubbletea program that collects
// provider + model + API key (+ base_url for ollama), writes the config,
// then returns the loaded *config.Config so tui_cmd.go can continue
// into the main chat TUI without a second process.
//
// Architecture:
//   runTUICommand → config.Load → ErrNoConfig → RunSetupTUI(cfgPath)
//   RunSetupTUI → requireTTY → tea.NewProgram(setupModel) → done
//   → config written → config.Load → returns *config.Config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/config"
	"daimon/internal/setup"
)

// errSetupAborted is the sentinel returned when the user quits before completing.
var errSetupAborted = errors.New("setup cancelled")

// setupStep is the multi-step flow state.
type setupStep int

const (
	stepProvider    setupStep = iota // choose provider from list
	stepCredentials                  // enter model + API key (+ base_url for ollama)
	stepRAGEnable                    // "Enable RAG?" yes/no pick
	stepRAGProvider                  // choose embedding provider (openai | gemini)
	stepRAGCreds                     // enter embedding model + API key
	stepDone                         // write completed or errored
)

// ragSetup holds the RAG-related inputs collected during setup.
// Passed to buildSetupConfig so the signature stays clean.
type ragSetup struct {
	enabled                 bool
	provider, model, apiKey string
}

// ragEmbeddingProviders is the ordered list of supported embedding providers.
var ragEmbeddingProviders = []string{"openai", "gemini"}

// ragEmbeddingDefaults maps a provider to its sensible default embedding model.
var ragEmbeddingDefaults = map[string]string{
	"openai": "text-embedding-3-small",
	"gemini": "text-embedding-004",
}

// ragOptLabels are the display labels for the RAGEnable step.
var ragOptLabels = []string{"No", "Yes"}

// setupWroteMsg is returned by writeConfigCmd after the write attempt.
type setupWroteMsg struct {
	path string
	err  error
}

// setupModel is the root tea.Model for the setup program.
// It is a SINGLE root model (no nested Elm sub-models).
type setupModel struct {
	styles tuiStyles

	step      setupStep
	cfgPath   string
	providers []string // = config.KnownProviders
	provIdx   int      // selected index into providers

	provider string // locked in when leaving stepProvider

	modelInput   textinput.Model // free-text model ID
	keyInput     textinput.Model // EchoPassword for API key (non-ollama only)
	baseURLInput textinput.Model // ollama base URL (ollama only)

	// fieldIdx: 0=model (both providers), 1=key (non-ollama) OR baseURL (ollama).
	// fieldCount is always 2; the second field's identity depends on the provider.
	fieldIdx int

	// RAG step state.
	ragOptIdx     int             // 0=No, 1=Yes at stepRAGEnable
	ragProviders  []string        // = ragEmbeddingProviders
	ragProvIdx    int             // selected index at stepRAGProvider
	ragProvider   string          // locked in when leaving stepRAGProvider
	embModelInput textinput.Model // free-text embedding model (optional)
	embKeyInput   textinput.Model // EchoPassword for embedding API key (required)

	width  int
	height int

	writtenPath string
	writeErr    error
	aborted     bool

	validationErr string // inline field validation message
}

// RunSetupTUI launches the embedded setup program.
// If the user aborts (ctrl+c before completing), it returns errSetupAborted.
// On success, the config has been written to disk; it is reloaded via config.Load.
func RunSetupTUI(cfgPath string) (*config.Config, error) {
	if err := requireTTY(os.Stdin); err != nil {
		return nil, err
	}

	s := newTuiStyles()
	modelIn := textinput.New()
	modelIn.Placeholder = "model ID"
	modelIn.CharLimit = 128

	keyIn := textinput.New()
	keyIn.Placeholder = "API key"
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.CharLimit = 256

	baseIn := textinput.New()
	baseIn.Placeholder = "http://localhost:11434"
	baseIn.SetValue("http://localhost:11434")
	baseIn.CharLimit = 256

	embModelIn := textinput.New()
	embModelIn.Placeholder = "embedding model (optional)"
	embModelIn.CharLimit = 128

	embKeyIn := textinput.New()
	embKeyIn.Placeholder = "embedding API key"
	embKeyIn.EchoMode = textinput.EchoPassword
	embKeyIn.CharLimit = 256

	m := setupModel{
		styles:        s,
		step:          stepProvider,
		cfgPath:       cfgPath,
		providers:     config.KnownProviders,
		provIdx:       0,
		modelInput:    modelIn,
		keyInput:      keyIn,
		baseURLInput:  baseIn,
		ragProviders:  ragEmbeddingProviders,
		embModelInput: embModelIn,
		embKeyInput:   embKeyIn,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalRaw, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("setup tui: %w", err)
	}

	final := finalRaw.(setupModel)

	if err := resolveSetupError(final); err != nil {
		return nil, err
	}

	return config.Load(final.writtenPath)
}

// IsSetupAborted reports whether the error is the user-abort sentinel.
func IsSetupAborted(err error) bool {
	return errors.Is(err, errSetupAborted)
}

// resolveSetupError selects the appropriate error from the final setup model.
// writeErr always takes precedence over aborted: if WriteConfig failed and the
// user pressed ctrl+c on the error screen, the real cause is the write failure.
func resolveSetupError(final setupModel) error {
	if final.writeErr != nil {
		return fmt.Errorf("setup tui: write config: %w", final.writeErr)
	}
	if final.aborted {
		return errSetupAborted
	}
	return nil
}

// ─── tea.Model interface ────────────────────────────────────────────────────

func (m setupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case setupWroteMsg:
		if msg.err != nil {
			m.writeErr = msg.err
			m.step = stepDone
			return m, nil
		}
		m.writtenPath = msg.path
		return m, tea.Quit

	case tea.KeyMsg:
		// Global: ctrl+c always aborts
		if msg.Type == tea.KeyCtrlC {
			m.aborted = true
			return m, tea.Quit
		}

		switch m.step {
		case stepProvider:
			return m.updateProvider(msg)
		case stepCredentials:
			return m.updateCredentials(msg)
		case stepRAGEnable:
			return m.updateRAGEnable(msg)
		case stepRAGProvider:
			return m.updateRAGProvider(msg)
		case stepRAGCreds:
			return m.updateRAGCreds(msg)
		case stepDone:
			// Any key on the error screen quits
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m setupModel) updateProvider(msg tea.KeyMsg) (setupModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.provIdx > 0 {
			m.provIdx--
		}
	case tea.KeyDown:
		if m.provIdx < len(m.providers)-1 {
			m.provIdx++
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			if m.provIdx > 0 {
				m.provIdx--
			}
		case "j":
			if m.provIdx < len(m.providers)-1 {
				m.provIdx++
			}
		}
	case tea.KeyEnter:
		m.provider = m.providers[m.provIdx]

		// Prefill model with the first catalog entry (if any)
		models := setup.ModelsForProvider(m.provider)
		if len(models) > 0 {
			m.modelInput.SetValue(models[0].ID)
		} else {
			m.modelInput.SetValue("")
		}
		m.fieldIdx = 0
		m.modelInput.Focus()
		m.keyInput.Blur()
		m.baseURLInput.Blur()
		m.validationErr = ""
		m.step = stepCredentials
	}
	return m, nil
}

func (m setupModel) updateCredentials(msg tea.KeyMsg) (setupModel, tea.Cmd) {
	// Both providers show exactly 2 fields:
	//   non-ollama: 0=model, 1=key
	//   ollama:     0=model, 1=baseURL
	ollama := m.provider == "ollama"
	const fieldCount = 2

	switch msg.Type {
	case tea.KeyEsc:
		m.step = stepProvider
		m.modelInput.Blur()
		m.keyInput.Blur()
		m.baseURLInput.Blur()
		return m, nil

	case tea.KeyTab, tea.KeyShiftTab:
		delta := 1
		if msg.Type == tea.KeyShiftTab {
			delta = -1
		}
		m.fieldIdx = (m.fieldIdx + delta + fieldCount) % fieldCount
		m = m.focusField(ollama)
		return m, textinput.Blink

	case tea.KeyEnter:
		model := strings.TrimSpace(m.modelInput.Value())
		key := strings.TrimSpace(m.keyInput.Value())

		if model == "" {
			m.validationErr = "model ID cannot be empty"
			return m, nil
		}
		if !ollama && key == "" {
			m.validationErr = "API key cannot be empty"
			return m, nil
		}

		m.validationErr = ""
		// Advance to the RAG enable step instead of writing immediately.
		m.modelInput.Blur()
		m.keyInput.Blur()
		m.baseURLInput.Blur()
		m.ragOptIdx = 0 // default: No
		m.step = stepRAGEnable
		return m, nil
	}

	// Route keystrokes to the focused input.
	// fieldIdx is always in {0,1}; case 0=model, case 1=key(non-ollama)|baseURL(ollama).
	var cmd tea.Cmd
	switch m.fieldIdx {
	case 0:
		m.modelInput, cmd = m.modelInput.Update(msg)
	case 1:
		if ollama {
			m.baseURLInput, cmd = m.baseURLInput.Update(msg)
		} else {
			m.keyInput, cmd = m.keyInput.Update(msg)
		}
	}
	return m, cmd
}

// focusField applies Focus/Blur to the text inputs based on fieldIdx.
// fieldIdx is always in {0, 1}:
//   - 0 = model (both providers)
//   - 1 = key (non-ollama) OR baseURL (ollama)
//
// keyInput is never focused for ollama; baseURLInput is never focused for non-ollama.
func (m setupModel) focusField(ollama bool) setupModel {
	m.modelInput.Blur()
	m.keyInput.Blur()
	m.baseURLInput.Blur()

	switch m.fieldIdx {
	case 0:
		m.modelInput.Focus()
	case 1:
		if ollama {
			m.baseURLInput.Focus()
		} else {
			m.keyInput.Focus()
		}
	}
	return m
}

// writeConfigCmd is the IO Cmd — runs outside the model, returns a setupWroteMsg.
// Note: if the user sends ctrl+c while this cmd is in flight, the file may be
// written but RunSetupTUI returns errSetupAborted. This is an accepted
// bubbletea-v1 limitation; it self-heals on the next run (config.Load succeeds).
func writeConfigCmd(provider, model, apiKey, baseURL, cfgPath string, rag ragSetup) tea.Cmd {
	return func() tea.Msg {
		cfg := buildSetupConfig(provider, model, apiKey, baseURL, rag)

		writePath := cfgPath
		if writePath == "" {
			p, err := setup.DefaultConfigPath()
			if err != nil {
				return setupWroteMsg{err: err}
			}
			writePath = p
		}

		if err := setup.WriteConfig(writePath, cfg); err != nil {
			return setupWroteMsg{err: err}
		}
		return setupWroteMsg{path: writePath}
	}
}

// buildSetupConfig builds a *config.Config from the collected setup values.
// Pure function — no IO, fully unit-testable.
// Hardcodes channel.type="cli" and store.type="sqlite" (fixes file-vs-sqlite divergence).
func buildSetupConfig(provider, model, apiKey, baseURL string, rag ragSetup) *config.Config {
	cfg := &config.Config{}

	creds := config.ProviderCredentials{APIKey: apiKey}
	if baseURL != "" {
		creds.BaseURL = baseURL
	}
	cfg.Providers = map[string]config.ProviderCredentials{
		provider: creds,
	}

	cfg.Models = config.ModelsConfig{
		Default: config.ModelRef{
			Provider: provider,
			Model:    model,
		},
	}

	cfg.Channel.Type = "cli"

	cfg.Store.Type = "sqlite"
	cfg.Store.Path = "~/.daimon/data"

	auditEnabled := true
	cfg.Audit.Enabled = &auditEnabled
	cfg.Audit.Type = "sqlite"
	cfg.Audit.Path = "~/.daimon/audit"

	if rag.enabled {
		cfg.RAG.Enabled = true
		cfg.RAG.Embedding = config.RAGEmbeddingConf{
			Enabled:  true,
			Provider: rag.provider,
			Model:    rag.model,
			APIKey:   rag.apiKey,
		}
	}

	return cfg
}

// updateRAGEnable handles input at the stepRAGEnable step.
// n/N → disable RAG + write; y/Y → enable RAG + go to stepRAGProvider.
// up/down (or left/right) toggle the No/Yes selector; enter confirms.
// Esc goes back to stepCredentials.
func (m setupModel) updateRAGEnable(msg tea.KeyMsg) (setupModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.step = stepCredentials
		// Re-focus the last active field in credentials.
		m.fieldIdx = 0
		m.modelInput.Focus()
		return m, nil

	case tea.KeyUp, tea.KeyLeft:
		m.ragOptIdx = 0
	case tea.KeyDown, tea.KeyRight:
		m.ragOptIdx = 1

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "n", "N":
			// Disable RAG and go straight to write.
			provider := m.provider
			model := strings.TrimSpace(m.modelInput.Value())
			apiKey := strings.TrimSpace(m.keyInput.Value())
			baseURL := strings.TrimSpace(m.baseURLInput.Value())
			return m, writeConfigCmd(provider, model, apiKey, baseURL, m.cfgPath, ragSetup{})
		case "y", "Y":
			m.step = stepRAGProvider
			m.ragProvIdx = 0
			return m, nil
		}

	case tea.KeyEnter:
		if m.ragOptIdx == 0 {
			// No selected — write without RAG.
			provider := m.provider
			model := strings.TrimSpace(m.modelInput.Value())
			apiKey := strings.TrimSpace(m.keyInput.Value())
			baseURL := strings.TrimSpace(m.baseURLInput.Value())
			return m, writeConfigCmd(provider, model, apiKey, baseURL, m.cfgPath, ragSetup{})
		}
		// Yes selected — go to provider picker.
		m.step = stepRAGProvider
		m.ragProvIdx = 0
		return m, nil
	}

	return m, nil
}

// updateRAGProvider handles input at the stepRAGProvider step.
// up/down navigate the provider list; enter confirms and advances to stepRAGCreds.
func (m setupModel) updateRAGProvider(msg tea.KeyMsg) (setupModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.ragProvIdx > 0 {
			m.ragProvIdx--
		}
	case tea.KeyDown:
		if m.ragProvIdx < len(m.ragProviders)-1 {
			m.ragProvIdx++
		}
	case tea.KeyEnter:
		m.ragProvider = m.ragProviders[m.ragProvIdx]
		// Prefill embedding model with a sensible default.
		if def, ok := ragEmbeddingDefaults[m.ragProvider]; ok {
			m.embModelInput.SetValue(def)
		} else {
			m.embModelInput.SetValue("")
		}
		m.fieldIdx = 0
		m.embModelInput.Focus()
		m.embKeyInput.Blur()
		m.validationErr = ""
		m.step = stepRAGCreds
		return m, textinput.Blink
	case tea.KeyEsc:
		m.step = stepRAGEnable
		return m, nil
	}
	return m, nil
}

// updateRAGCreds handles input at the stepRAGCreds step.
// Two fields: 0=embModel (optional), 1=embKey (required).
// Tab/ShiftTab cycle; enter validates (embKey non-empty) → write.
func (m setupModel) updateRAGCreds(msg tea.KeyMsg) (setupModel, tea.Cmd) {
	const fieldCount = 2

	switch msg.Type {
	case tea.KeyEsc:
		m.step = stepRAGProvider
		m.embModelInput.Blur()
		m.embKeyInput.Blur()
		return m, nil

	case tea.KeyTab, tea.KeyShiftTab:
		delta := 1
		if msg.Type == tea.KeyShiftTab {
			delta = -1
		}
		m.fieldIdx = (m.fieldIdx + delta + fieldCount) % fieldCount
		m = m.focusRAGCredsField()
		return m, textinput.Blink

	case tea.KeyEnter:
		embKey := strings.TrimSpace(m.embKeyInput.Value())
		if embKey == "" {
			m.validationErr = "embedding API key cannot be empty"
			return m, nil
		}
		m.validationErr = ""
		provider := m.provider
		model := strings.TrimSpace(m.modelInput.Value())
		apiKey := strings.TrimSpace(m.keyInput.Value())
		baseURL := strings.TrimSpace(m.baseURLInput.Value())
		rag := ragSetup{
			enabled:  true,
			provider: m.ragProvider,
			model:    strings.TrimSpace(m.embModelInput.Value()),
			apiKey:   embKey,
		}
		return m, writeConfigCmd(provider, model, apiKey, baseURL, m.cfgPath, rag)
	}

	// Route keystrokes to the focused field.
	var cmd tea.Cmd
	switch m.fieldIdx {
	case 0:
		m.embModelInput, cmd = m.embModelInput.Update(msg)
	case 1:
		m.embKeyInput, cmd = m.embKeyInput.Update(msg)
	}
	return m, cmd
}

// focusRAGCredsField applies Focus/Blur to the RAG creds inputs based on fieldIdx.
// fieldIdx is always in {0, 1}: 0=embModel, 1=embKey.
func (m setupModel) focusRAGCredsField() setupModel {
	m.embModelInput.Blur()
	m.embKeyInput.Blur()

	switch m.fieldIdx {
	case 0:
		m.embModelInput.Focus()
	case 1:
		m.embKeyInput.Focus()
	}
	return m
}

// ─── View ──────────────────────────────────────────────────────────────────

func (m setupModel) View() string {
	switch m.step {
	case stepProvider:
		return m.viewProvider()
	case stepCredentials:
		return m.viewCredentials()
	case stepRAGEnable:
		return m.viewRAGEnable()
	case stepRAGProvider:
		return m.viewRAGProvider()
	case stepRAGCreds:
		return m.viewRAGCreds()
	case stepDone:
		return m.viewDone()
	default:
		return ""
	}
}

func (m setupModel) viewProvider() string {
	s := m.styles
	var sb strings.Builder

	sb.WriteString(s.accent.Render("⫶ daimon — first-run setup"))
	sb.WriteString("\n\n")
	sb.WriteString(s.label.Render("Choose a provider:"))
	sb.WriteString("\n\n")

	for i, p := range m.providers {
		line := "  " + p
		if i == m.provIdx {
			line = s.selected.Render("▶ " + p)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(s.hint.Render("↑/↓  navigate • enter  select • ctrl+c  quit"))
	return sb.String()
}

func (m setupModel) viewCredentials() string {
	s := m.styles
	ollama := m.provider == "ollama"

	var sb strings.Builder
	sb.WriteString(s.accent.Render("⫶ daimon — first-run setup"))
	sb.WriteString("\n\n")
	sb.WriteString(s.label.Render("Provider: ") + s.accent.Render(m.provider))
	sb.WriteString("\n\n")

	// Model field
	label := s.dimLabel.Render("Model ID")
	if m.fieldIdx == 0 {
		label = s.label.Render("Model ID")
	}
	sb.WriteString(label + "\n")
	sb.WriteString(m.modelInput.View())
	sb.WriteString("\n\n")

	if ollama {
		// Base URL field
		label2 := s.dimLabel.Render("Base URL")
		if m.fieldIdx == 1 {
			label2 = s.label.Render("Base URL")
		}
		sb.WriteString(label2 + "\n")
		sb.WriteString(m.baseURLInput.View())
		sb.WriteString("\n\n")
	} else {
		// API key field
		label2 := s.dimLabel.Render("API Key")
		if m.fieldIdx == 1 {
			label2 = s.label.Render("API Key")
		}
		sb.WriteString(label2 + "\n")
		sb.WriteString(m.keyInput.View())
		sb.WriteString("\n\n")
	}

	if m.validationErr != "" {
		sb.WriteString(s.errStyle.Render("✗ " + m.validationErr))
		sb.WriteString("\n\n")
	}

	sb.WriteString(s.hint.Render("tab  next field • shift+tab  prev field • enter  save • esc  back • ctrl+c  quit"))
	return sb.String()
}

func (m setupModel) viewRAGEnable() string {
	s := m.styles
	var sb strings.Builder

	sb.WriteString(s.accent.Render("⫶ daimon — first-run setup"))
	sb.WriteString("\n\n")
	sb.WriteString(s.label.Render("Enable RAG (semantic search over your docs/memory)?"))
	sb.WriteString("\n\n")

	for i, opt := range ragOptLabels {
		line := "  " + opt
		if i == m.ragOptIdx {
			line = s.selected.Render("▶ " + opt)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(s.hint.Render("↑/↓  toggle • enter  confirm • y  enable • n  skip • esc  back • ctrl+c  quit"))
	return sb.String()
}

func (m setupModel) viewRAGProvider() string {
	s := m.styles
	var sb strings.Builder

	sb.WriteString(s.accent.Render("⫶ daimon — first-run setup"))
	sb.WriteString("\n\n")
	sb.WriteString(s.label.Render("Choose embedding provider:"))
	sb.WriteString("\n\n")

	for i, p := range m.ragProviders {
		line := "  " + p
		if i == m.ragProvIdx {
			line = s.selected.Render("▶ " + p)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(s.hint.Render("↑/↓  navigate • enter  select • esc  back • ctrl+c  quit"))
	return sb.String()
}

func (m setupModel) viewRAGCreds() string {
	s := m.styles
	var sb strings.Builder

	sb.WriteString(s.accent.Render("⫶ daimon — first-run setup"))
	sb.WriteString("\n\n")
	sb.WriteString(s.label.Render("Embedding provider: ") + s.accent.Render(m.ragProvider))
	sb.WriteString("\n\n")

	// Embedding model field (optional).
	embModelLabel := s.dimLabel.Render("Embedding Model (optional)")
	if m.fieldIdx == 0 {
		embModelLabel = s.label.Render("Embedding Model (optional)")
	}
	sb.WriteString(embModelLabel + "\n")
	sb.WriteString(m.embModelInput.View())
	sb.WriteString("\n\n")

	// Embedding API key field (required).
	embKeyLabel := s.dimLabel.Render("Embedding API Key")
	if m.fieldIdx == 1 {
		embKeyLabel = s.label.Render("Embedding API Key")
	}
	sb.WriteString(embKeyLabel + "\n")
	sb.WriteString(m.embKeyInput.View())
	sb.WriteString("\n\n")

	if m.validationErr != "" {
		sb.WriteString(s.errStyle.Render("✗ " + m.validationErr))
		sb.WriteString("\n\n")
	}

	sb.WriteString(s.hint.Render("tab  next field • shift+tab  prev field • enter  save • esc  back • ctrl+c  quit"))
	return sb.String()
}

func (m setupModel) viewDone() string {
	s := m.styles
	// stepDone is only reached on write error (success sends tea.Quit immediately).
	return s.errStyle.Render("✗ setup failed: "+m.writeErr.Error()) +
		"\n\n" + s.hint.Render("any key to quit")
}
