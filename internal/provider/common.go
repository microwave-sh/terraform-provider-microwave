package provider

// timeFormat is the canonical timestamp format used across resources for
// computed created_at / updated_at attributes. RFC 3339 matches what the
// Microwave API returns and what users expect to see in state.
const timeFormat = "2006-01-02T15:04:05Z07:00"
