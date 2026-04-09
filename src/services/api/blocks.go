package api

// TextBlock is a minimal content block used in tool_result content arrays.
// We use this to get closer to TS behavior where content may be block arrays.
type TextBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

