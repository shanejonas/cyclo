package application

import "unicode/utf8"

// sourceWorkspace tracks the transient interaction state of the source
// pane: cursor position, selection, and annotation drafting. It resets as
// a unit whenever the viewed file or function changes, so the operations
// below keep its fields consistent in one place instead of spreading
// individual assignments across the model.
type sourceWorkspace struct {
	qualityOffset         int
	sourceOffset          int
	sourceCursor          int
	lineSelection         *LineSelection
	visualSelectionActive bool
	activeAnnotationID    string
	annotating            bool
	annotationDraft       string
}

// clearSelection drops any active line selection.
func (w sourceWorkspace) clearSelection() sourceWorkspace {
	w.lineSelection = nil
	w.visualSelectionActive = false
	w.activeAnnotationID = ""
	return w
}

// withSelection replaces the line selection, clearing visual-select state.
func (w sourceWorkspace) withSelection(selection *LineSelection) sourceWorkspace {
	w.lineSelection = selection
	w.visualSelectionActive = false
	w.activeAnnotationID = ""
	return w
}

// selectLines points the workspace at the given source line range, where
// base is the first line of the viewed function.
func (w sourceWorkspace) selectLines(start, end, base int, text string) sourceWorkspace {
	w.sourceOffset = start - base
	w.sourceCursor = end - base
	return w.withSelection(&LineSelection{AnchorLine: start, StartLine: start, EndLine: end, Text: text})
}

// focusAnnotation points the workspace at an annotation, moving the cursor
// to its location when locate is set.
func (w sourceWorkspace) focusAnnotation(annotation Annotation, locate bool) sourceWorkspace {
	w = w.clearSelection()
	w.activeAnnotationID = annotation.ID
	if locate {
		w.sourceCursor = annotation.EndLine - annotation.FunctionLine
	}
	return w
}

// stopAnnotating leaves annotation composition mode, keeping the draft.
func (w sourceWorkspace) stopAnnotating() sourceWorkspace {
	w.annotating = false
	return w
}

// cancelDraft abandons the annotation being composed.
func (w sourceWorkspace) cancelDraft() sourceWorkspace {
	w = w.stopAnnotating()
	w.annotationDraft = ""
	return w
}

// trimDraft deletes the last rune of the draft.
func (w sourceWorkspace) trimDraft() sourceWorkspace {
	w.annotationDraft = trimLastRune(w.annotationDraft)
	return w
}

// typeDraft appends text to the draft within the length limit.
func (w sourceWorkspace) typeDraft(text string) sourceWorkspace {
	draft := w.annotationDraft + text
	if text != "" && utf8.RuneCountInString(draft) <= maximumAnnotationLength {
		w.annotationDraft = draft
	}
	return w
}

// finishDraft records a saved annotation and clears the composition state.
func (w sourceWorkspace) finishDraft(id string) sourceWorkspace {
	w = w.clearSelection()
	w.activeAnnotationID = id
	w.annotationDraft = ""
	return w
}
