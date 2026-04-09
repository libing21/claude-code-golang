package attachments

// RelevantMemoryAttachment mirrors TS Attachment type 'relevant_memories' element.
// Header is precomputed at creation time to keep rendered bytes stable across turns.
type RelevantMemoryAttachment struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	MtimeMs int64  `json:"mtimeMs"`
	Header  string `json:"header,omitempty"`
	Limit   *int   `json:"limit,omitempty"`
}

type PlanModeAttachment struct {
	ReminderType string `json:"reminderType"` // "full" | "sparse"
	IsSubAgent   bool   `json:"isSubAgent,omitempty"`
	PlanFilePath string `json:"planFilePath"`
	PlanExists   bool   `json:"planExists"`
}

type PlanModeReentryAttachment struct {
	PlanFilePath string `json:"planFilePath"`
}

type PlanModeExitAttachment struct {
	PlanFilePath string `json:"planFilePath"`
	PlanExists   bool   `json:"planExists"`
}

type AutoModeAttachment struct {
	ReminderType string `json:"reminderType"` // "full" | "sparse"
}

type DeferredToolsDeltaAttachment struct {
	AddedNames   []string `json:"addedNames"`
	AddedLines   []string `json:"addedLines"`
	RemovedNames []string `json:"removedNames"`
}

type AgentListingDeltaAttachment struct {
	AddedTypes          []string `json:"addedTypes"`
	AddedLines          []string `json:"addedLines"`
	RemovedTypes        []string `json:"removedTypes"`
	IsInitial           bool     `json:"isInitial"`
	ShowConcurrencyNote bool     `json:"showConcurrencyNote"`
}
