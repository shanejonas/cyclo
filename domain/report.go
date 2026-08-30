package domain

type Report struct {
	Root             string
	DiffBase         string
	Files            []File
	Functions        int
	Total            int
	Average          float64
	CognitiveTotal   int
	CognitiveAverage float64
}

type File struct {
	Path             string
	Functions        []Function
	Total            int
	Peak             int
	Average          float64
	CognitiveTotal   int
	CognitivePeak    int
	CognitiveAverage float64
}

type Function struct {
	Package               string
	Name                  string
	Complexity            int
	CyclomaticDiagnostics []CyclomaticDiagnostic
	CognitiveComplexity   int
	CognitiveDiagnostics  []CognitiveDiagnostic
	Line                  int
	EndLine               int
	Column                int
	Source                string
	DiffLines             []DiffLine
}

type DiffLine struct {
	Kind    DiffKind
	OldLine int
	NewLine int
	Text    string
}

type DiffKind string

const (
	DiffAdded   DiffKind = "added"
	DiffDeleted DiffKind = "deleted"
)

type CyclomaticDiagnostic struct {
	Kind   string `json:"kind"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type CognitiveDiagnostic struct {
	Increment int    `json:"increment"`
	Nesting   int    `json:"nesting"`
	Kind      string `json:"kind"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
}
