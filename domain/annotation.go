package domain

type Annotation struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Function     string `json:"function"`
	FunctionLine int    `json:"functionLine"`
	StartLine    int    `json:"startLine"`
	EndLine      int    `json:"endLine"`
	Message      string `json:"message"`
	Text         string `json:"text"`
}
