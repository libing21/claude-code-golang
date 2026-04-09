package prompts

// InsertAfterBoundary inserts a section immediately after SYSTEM_PROMPT_DYNAMIC_BOUNDARY.
// If boundary is not present, it appends to the end.
func InsertAfterBoundary(systemPrompt []string, section string) []string {
	if section == "" {
		return systemPrompt
	}
	for i, s := range systemPrompt {
		if s == SYSTEM_PROMPT_DYNAMIC_BOUNDARY {
			out := make([]string, 0, len(systemPrompt)+1)
			out = append(out, systemPrompt[:i+1]...)
			out = append(out, section)
			out = append(out, systemPrompt[i+1:]...)
			return out
		}
	}
	out := append([]string{}, systemPrompt...)
	out = append(out, section)
	return out
}

