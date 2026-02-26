package nats

const (
	StreamNotificationsDLQ  = "NOTIFICATIONS_DLQ"
	StreamActivityLog       = "activity_log"
	StreamThreadMetadata    = "thread_metadata"
	StreamThreadAccess      = "thread_access"
	StreamThreadValidations = "thread_validations"
	StreamStepState         = "step_state"
	StreamOutboxTriggers    = "OUTBOX_TRIGGERS"

	// Subject patterns and prefixes
	SubjectNotificationsUser = "notifications.user.>"
	SubjectNotificationsDLQ  = "notifications.dlq.>"
	SubjectActivityLog       = "activity.log"
	SubjectThreadMetadata    = "metadata.thread"
	SubjectThreadAccess      = "access.thread"
	SubjectThreadValidations = "validations.thread"
	SubjectStepState         = "state.step"
	SubjectOutboxTrigger     = "outbox.trigger"

	PrefixNotificationsUser = "notifications.user"
)
