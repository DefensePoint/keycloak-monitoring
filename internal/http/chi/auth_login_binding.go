package chi

// logPrefix returns the start of a state for the log. The state comes from the
// request, so it can be shorter than the prefix.
func logPrefix(state string) string {
	if len(state) > 8 {
		return state[:8] + "..."
	}
	return state
}
