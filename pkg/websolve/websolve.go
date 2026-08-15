package websolve

import (
	"backpack-brawl-solver/internal/solver"
	internalwebsolve "backpack-brawl-solver/internal/websolve"
)

type Options = internalwebsolve.Options
type Metadata = internalwebsolve.Metadata
type Result = internalwebsolve.Result
type Request = internalwebsolve.Request
type ProgressReporter = solver.ProgressReporter

func SolveScenarioJSON(input []byte) ([]byte, error) {
	return internalwebsolve.SolveScenarioJSON(input)
}

func SolveScenarioJSONWithProgress(input []byte, progressReporter ProgressReporter) ([]byte, error) {
	return internalwebsolve.SolveScenarioJSONWithProgress(input, progressReporter)
}

func SolveScenarioJSONWithOptions(input []byte, options Options) (Result, error) {
	return internalwebsolve.SolveScenarioJSONWithOptions(input, options)
}
