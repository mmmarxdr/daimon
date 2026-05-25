package notify

// Origin identifies who generated the event — prevents notification loops.
type Origin string

const (
	// OriginCron is used for events emitted by the cron scheduler subsystem.
	OriginCron Origin = "cron"

	// OriginAgent is used for events emitted by the agent loop.
	OriginAgent Origin = "agent"

	// OriginNotification is used for events emitted by the NotificationSender.
	// Events with this origin are dropped by the bus worker to prevent loops.
	OriginNotification Origin = "notification"
)

// Event type constants — the known event type set.
const (
	// Cron subsystem events.
	EventCronJobFired     = "cron.job.fired"
	EventCronJobCompleted = "cron.job.completed"
	EventCronJobFailed    = "cron.job.failed"

	// Agent turn lifecycle events.
	EventTurnStarted      = "agent.turn.started"
	EventTurnCompleted    = "agent.turn.completed"
	EventContextCompacted = "agent.context.compacted"

	// Notification system internal audit events — never matched by rules.
	// Events with Origin == OriginNotification are dropped by the bus worker.
	EventNotificationSent   = "notification.sent"
	EventNotificationFailed = "notification.failed"

	// Subagent lifecycle events — emitted by SubagentManager per design §2.6.
	// All three use OriginAgent and the Meta map for structured payload fields.
	EventSubagentSpawned   = "agent.subagent.spawned"
	EventSubagentCompleted = "agent.subagent.completed"
	EventSubagentFailed    = "agent.subagent.failed"

	// Todolist mutation event (todolist-tool change, REQ-7).
	// Emitted by the agent Mutate callback after a successful create or update.
	// Never emitted by todo_list (read-only). Must NOT be in StreamingSkipSet.
	EventTodolistChanged = "agent.todolist.changed"

	// Streaming / tool-lifecycle event types (agent-stream-events change, REQ-12).
	//
	// Bus-routed events (structured boundaries, ~5–50 per turn):
	EventReasoningStart = "agent.reasoning.start" // first ReasoningDelta of a contiguous span
	EventReasoningEnd   = "agent.reasoning.end"   // last ReasoningDelta or StreamEventDone closes span
	EventToolStart      = "agent.tool.start"      // before tool execution (processMessage)
	EventToolEnd        = "agent.tool.end"        // after tool execution (processMessage)
	EventTokensUsage    = "agent.tokens.usage"    // once per turn at turn end (processMessage)

	// Interface-only events (high-frequency; NOT on the bus — via StreamWriter/TelemetryEmitter):
	EventMessageChunk   = "agent.message.chunk"   // text delta via StreamWriter.WriteChunk
	EventReasoningDelta = "agent.reasoning.delta" // reasoning delta via StreamWriter.WriteReasoning
	EventToolDelta      = "agent.tool.delta"      // tool input assembly delta via TelemetryEmitter
)

// KnownEventTypes is the set of valid event types for rule validation.
// notification.sent and notification.failed are intentionally excluded —
// they are never matched by rules (OriginNotification guard drops them).
// Interface-only streaming types (agent.message.chunk, agent.reasoning.delta,
// agent.tool.delta) are also excluded — they never travel on the bus.
var KnownEventTypes = map[string]bool{
	EventCronJobFired:     true,
	EventCronJobCompleted: true,
	EventCronJobFailed:    true,
	EventTurnStarted:      true,
	EventTurnCompleted:    true,
	EventContextCompacted: true,

	// Subagent lifecycle events.
	EventSubagentSpawned:   true,
	EventSubagentCompleted: true,
	EventSubagentFailed:    true,

	// Todolist mutation event (todolist-tool, REQ-7).
	EventTodolistChanged: true,

	// Bus-routed streaming boundary events (REQ-12).
	// These are registered for completeness / validation tooling but are
	// skipped by the rules engine via StreamingSkipSet (REQ-12.3).
	EventReasoningStart: true,
	EventReasoningEnd:   true,
	EventToolStart:      true,
	EventToolEnd:        true,
	EventTokensUsage:    true,
}

// StreamingSkipSet contains the new bus-routed event types that the rules
// engine must skip in V1 (REQ-12.3). They are registered in KnownEventTypes
// for validation tooling but MUST NOT trigger user notification rules.
var StreamingSkipSet = map[string]bool{
	EventReasoningStart: true,
	EventReasoningEnd:   true,
	EventToolStart:      true,
	EventToolEnd:        true,
	EventTokensUsage:    true,
}
