package guard

// Inverted: the happy path is trapped in the if body.
func parseFlag(lastArg string) (string, string, error) {
	var flagName string
	if len(lastArg) > 0 && lastArg[0] == '-' {
		flagName = lastArg[1:]
		lastArg = ""
		_ = flagName
	} else {
		return "", "", nil
	}
	return flagName, lastArg, nil
}

// Already a guard clause: must not be flagged.
func parseOther(lastArg string) (string, error) {
	if len(lastArg) == 0 {
		return "", nil
	}
	out := lastArg + "!"
	return out, nil
}
