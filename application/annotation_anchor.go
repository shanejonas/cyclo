package application

import (
	"fmt"
	"strings"

	"github.com/shanejonas/cyclo/domain"
)

func (m Model) relocateAnnotations() Model {
	m.annotations = append([]Annotation(nil), m.annotations...)
	for index, annotation := range m.annotations {
		for _, file := range m.report.Files {
			if file.Path != annotation.Path {
				continue
			}
			function, ok := annotationFunction(file.Functions, annotation)
			if ok {
				m.annotations[index] = relocateAnnotation(annotation, function)
			}
		}
	}
	return m
}

func annotationFunction(functions []domain.Function, annotation Annotation) (domain.Function, bool) {
	var match domain.Function
	count := 0
	for _, function := range functions {
		if function.Name == annotation.Function {
			match = function
			count++
		}
	}
	if count == 1 {
		return match, true
	}
	count = 0
	for _, function := range functions {
		_, ok := annotationTextLine(function, annotation.Text)
		if ok {
			match = function
			count++
		}
	}
	return match, count == 1
}

func annotationTextLine(function domain.Function, text string) (int, bool) {
	if strings.TrimSpace(text) == "" {
		return 0, false
	}
	source := "\n" + normalizedSourceText(function.Source) + "\n"
	needle := "\n" + normalizedSourceText(text) + "\n"
	index := strings.Index(source, needle)
	if index < 0 || strings.Contains(source[index+1:], needle) {
		return 0, false
	}
	return function.Line + strings.Count(source[:index], "\n"), true
}

func relocateAnnotation(annotation Annotation, function domain.Function) Annotation {
	start := function.Line + annotation.StartLine - annotation.FunctionLine
	end := function.Line + annotation.EndLine - annotation.FunctionLine
	line, ok := annotationTextLine(function, annotation.Text)
	if ok {
		start = line
		end = start + strings.Count(annotation.Text, "\n")
	}
	last := function.Line + strings.Count(function.Source, "\n")
	start = min(max(start, function.Line), last)
	end = min(max(end, start), last)
	return Annotation{
		ID:           annotation.ID,
		Path:         annotation.Path,
		Function:     function.Name,
		FunctionLine: function.Line,
		StartLine:    start,
		EndLine:      end,
		Message:      annotation.Message,
		Text:         annotation.Text,
	}
}

func annotationMatchesFunction(annotation Annotation, function domain.Function) bool {
	return annotation.Function == function.Name && annotation.FunctionLine == function.Line
}

func (m Model) annotationNavigationLabel() string {
	if len(m.annotations) == 0 {
		return " notes (none)"
	}
	position, unmatched := m.annotationNavigationPosition()
	label := fmt.Sprintf(" notes (%d", len(m.annotations))
	if position > 0 {
		label = fmt.Sprintf(" notes (%d/%d", position, len(m.annotations))
	}
	if unmatched > 0 {
		label += fmt.Sprintf("; %d unmatched", unmatched)
	}
	return label + ")"
}

func (m Model) annotationNavigationPosition() (position int, unmatched int) {
	for index, target := range m.annotationTargets() {
		if target.ID == m.activeAnnotationID {
			position = index + 1
		}
		if target.functionIndex < 0 {
			unmatched++
		}
	}
	return position, unmatched
}
