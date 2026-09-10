package domain

// User is the authenticated account's Spotify profile, as returned by
// GET /v1/me. Only the fields the CLI currently displays are kept here;
// add more as features need them.
type User struct {
	ID          string
	DisplayName string
	Email       string
	Product     string // subscription tier, e.g. "premium", "free"
}
