package execution

import (
	"github.com/divilla/apihydra/internal/domain"
)

func prepareDirectory(dir *domain.Directory) {
	dir.RuntimeSteps = cloneStepGroups(dir.ResolvedSteps)
	for _, child := range dir.Children {
		prepareDirectory(child)
	}
}

func cloneStepGroups(groups [][]domain.Step) [][]domain.Step {
	if groups == nil {
		return nil
	}

	cloned := make([][]domain.Step, len(groups))
	for groupIndex, group := range groups {
		if group == nil {
			continue
		}
		cloned[groupIndex] = make([]domain.Step, len(group))
		for stepIndex := range group {
			cloned[groupIndex][stepIndex] = cloneStep(group[stepIndex])
		}
	}
	return cloned
}

func cloneStep(step domain.Step) domain.Step {
	cloned := step
	cloned.Vars = cloneMap(step.Vars)
	cloned.Request.Defaults.Headers = cloneMap(step.Request.Defaults.Headers)
	cloned.Request.Defaults.DisableCookies = cloneBool(step.Request.Defaults.DisableCookies)
	cloned.Response.Capture = cloneMap(step.Response.Capture)
	if step.Response.ExpectedTypes != nil {
		cloned.Response.ExpectedTypes = make(map[string][]string, len(step.Response.ExpectedTypes))
		for selector, expected := range step.Response.ExpectedTypes {
			if expected == nil {
				cloned.Response.ExpectedTypes[selector] = nil
				continue
			}
			clonedExpected := make([]string, len(expected))
			copy(clonedExpected, expected)
			cloned.Response.ExpectedTypes[selector] = clonedExpected
		}
	}
	return cloned
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	cloned := make(map[K]V, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneBool(source *bool) *bool {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}
