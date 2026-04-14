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
)

// KnownEventTypes is the set of valid event types for rule validation.
// notification.sent and notification.failed are intentionally excluded —
// they are never matched by rules (OriginNotification guard drops them).
var KnownEventTypes = map[string]bool{
	EventCronJobFired:     true,
	EventCronJobCompleted: true,
	EventCronJobFailed:    true,
	EventTurnStarted:      true,
	EventTurnCompleted:    true,
	EventContextCompacted: true,
}
