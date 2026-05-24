package tool

import "context"

// effectiveCwdKey is the unexported context key for the per-turn effective
// working directory. It is injected by the agent loop at the executeWithRecover
// call site when a per-(channelID, senderID) override exists, so the shell tool
// uses the override without needing a direct reference to the agent.
type effectiveCwdKey struct{}

// WithEffectiveCwd returns a new context carrying the given working directory.
// The shell tool reads this via EffectiveCwdFromCtx and uses it as cmd.Dir,
// falling back to the static ShellToolConfig.WorkingDir when absent.
func WithEffectiveCwd(ctx context.Context, cwd string) context.Context {
	return context.WithValue(ctx, effectiveCwdKey{}, cwd)
}

// EffectiveCwdFromCtx extracts the effective working directory from ctx.
// Returns ("", false) when no value was set (shell tool falls back to static config).
func EffectiveCwdFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(effectiveCwdKey{}).(string)
	return v, ok
}
