package notify

import (
	"testing"
	"time"

	"daimon/internal/config"
)

// TestRulesEngine_SkipsStreamingEventTypes (REQ-12.3) — a rule matching
// agent.tool.start must NOT be triggered when that event type is in
// StreamingSkipSet.
func TestRulesEngine_SkipsStreamingEventTypes(t *testing.T) {
	rules := []config.NotificationRule{
		newRule("r-skip", EventToolStart, "", 0),
	}
	engine, cs := buildEngine(t, rules)

	// Emit one agent.tool.start event — the rules engine must skip it.
	engine.Handle(Event{
		Type:      EventToolStart,
		Origin:    OriginAgent,
		ChannelID: "test",
		Timestamp: time.Now(),
	})

	// Give goroutines a chance to fire if the skip is broken.
	time.Sleep(100 * time.Millisecond)

	if cs.count() != 0 {
		t.Errorf("rules engine fired for a StreamingSkipSet event type %q — expected 0 sends, got %d",
			EventToolStart, cs.count())
	}
}

// TestRulesEngine_SkipsAllFiveStreamingTypes verifies the skip set applies to
// all five bus-routed streaming event types, not just agent.tool.start.
func TestRulesEngine_SkipsAllFiveStreamingTypes(t *testing.T) {
	skipped := []string{
		EventToolStart,
		EventToolEnd,
		EventReasoningStart,
		EventReasoningEnd,
		EventTokensUsage,
	}

	for _, evType := range skipped {
		evType := evType // capture
		t.Run(evType, func(t *testing.T) {
			rules := []config.NotificationRule{
				newRule("r-skip-"+evType, evType, "", 0),
			}
			engine, cs := buildEngine(t, rules)

			engine.Handle(Event{
				Type:      evType,
				Origin:    OriginAgent,
				ChannelID: "test",
				Timestamp: time.Now(),
			})

			time.Sleep(50 * time.Millisecond)

			if cs.count() != 0 {
				t.Errorf("rules engine fired for streaming type %q — expected skip, got %d sends", evType, cs.count())
			}
		})
	}
}

// TestRulesEngine_NonStreamingTypesStillFire confirms that the skip set does
// not block normal cron/turn events.
func TestRulesEngine_NonStreamingTypesStillFire(t *testing.T) {
	rules := []config.NotificationRule{
		newRule("r-cron", EventCronJobCompleted, "", 0),
	}
	engine, cs := buildEngine(t, rules)
	engine.Handle(Event{
		Type:      EventCronJobCompleted,
		Origin:    OriginCron,
		ChannelID: "test",
		Timestamp: time.Now(),
	})
	// The test is primarily about the skip-set not over-blocking; since buildEngine
	// routes through countingSender we just verify no panic occurred. The companion
	// TestRules_MatchByEventType already covers positive firing.
	waitForCalls(t, cs, 1, 500*time.Millisecond)
}
