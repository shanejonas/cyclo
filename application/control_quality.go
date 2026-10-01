package application

import (
	"encoding/json"

	"github.com/shanejonas/cyclo/domain"
)

func qualitySummary(analysis *domain.QualityAnalysis) *domain.QualitySummary {
	if analysis == nil {
		return nil
	}
	if analysis.Report == nil {
		return &domain.QualitySummary{Status: analysis.Status, Error: analysis.Error}
	}
	findings := len(analysis.Report.Diagnostics)
	stats := analysis.Report.Summary
	return &domain.QualitySummary{
		Status:   analysis.Status,
		Error:    analysis.Error,
		Findings: &findings,
		Summary:  &stats,
	}
}

func parseSetDetailsView(params json.RawMessage) (controlCommand, *controlError) {
	values := struct {
		View string `json:"view"`
	}{}
	if err := decodeNamedParams(params, &values); err != nil {
		return controlCommand{}, err
	}
	if values.View != "source" && values.View != "quality" {
		return controlCommand{}, invalidParams("view must be source or quality")
	}
	return controlCommand{action: setDetailsViewAction, view: values.View}, nil
}
