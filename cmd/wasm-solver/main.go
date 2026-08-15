//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"backpack-brawl-solver/internal/render"
	"backpack-brawl-solver/internal/solver"
	"backpack-brawl-solver/internal/websolve"
)

func main() {
	js.Global().Set("solveScenario", js.FuncOf(solveScenario))
	select {}
}

func solveScenario(_ js.Value, args []js.Value) any {
	if len(args) < 1 || len(args) > 2 {
		return errorJSON("solveScenario expects a JSON string and optional progress callback")
	}
	var reporter solver.ProgressReporter
	if len(args) == 2 && args[1].Type() == js.TypeFunction {
		progressCallback := args[1]
		reporter = func(snapshot solver.ProgressSnapshot) {
			progressCallback.Invoke(progressToJS(snapshot))
		}
	}
	output, err := websolve.SolveScenarioJSONWithProgress([]byte(args[0].String()), reporter)
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(output)
}

func progressToJS(snapshot solver.ProgressSnapshot) any {
	value := map[string]any{
		"phase":            snapshot.Phase,
		"nodes_explored":   float64(snapshot.NodesExplored),
		"elapsed_ms":       float64(snapshot.ElapsedMS),
		"nodes_per_second": snapshot.NodesPerSecond,
	}
	if snapshot.NodesTotal > 0 {
		value["nodes_total"] = float64(snapshot.NodesTotal)
		value["percent"] = snapshot.Percent
	}
	if snapshot.EtaMS > 0 {
		value["eta_ms"] = float64(snapshot.EtaMS)
	}
	if len(snapshot.PartialSolutions) > 0 {
		if content, err := render.SolutionsJSON(snapshot.PartialSolutions); err == nil {
			var parsed any
			if err := json.Unmarshal(content, &parsed); err == nil {
				value["partial_solutions"] = parsed
			}
		}
	}
	return value
}

func errorJSON(message string) string {
	content, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		return `{"error":"unknown error"}`
	}
	return string(content)
}
