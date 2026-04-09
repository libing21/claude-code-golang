package prompts

// ReplaceSection replaces all exact matches of from with to. If to is empty, removes the section.
func ReplaceSection(systemPrompt []string, from string, to string) []string {
	out := make([]string, 0, len(systemPrompt))
	for _, s := range systemPrompt {
		if s == from {
			if to != "" {
				out = append(out, to)
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

