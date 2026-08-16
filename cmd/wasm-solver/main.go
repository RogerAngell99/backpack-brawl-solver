//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/render"
	"backpack-brawl-solver/internal/scenario"
	"backpack-brawl-solver/internal/solver"
	"backpack-brawl-solver/internal/websolve"
)

var (
	preparedCatalog    model.Catalog
	preparedCatalogSet bool
)

func main() {
	js.Global().Set("solveScenario", js.FuncOf(solveScenario))
	js.Global().Set("installCatalog", js.FuncOf(installCatalog))
	js.Global().Set("solvePreparedScenario", js.FuncOf(solvePreparedScenario))
	select {}
}

func solveScenario(_ js.Value, args []js.Value) any {
	if len(args) < 1 || len(args) > 2 {
		return errorJSON("solveScenario expects a JSON string and optional progress callback")
	}
	if args[0].Type() != js.TypeString {
		return errorJSON("solveScenario expects a JSON string")
	}
	result, err := websolve.SolveScenarioJSONWithOptions([]byte(args[0].String()), wasmOptions(args))
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(result.JSON)
}

func installCatalog(_ js.Value, args []js.Value) any {
	if len(args) != 1 || args[0].Type() != js.TypeString {
		return errorJSON("installCatalog expects a catalog JSON string")
	}
	loadedCatalog, err := catalog.Parse([]byte(args[0].String()))
	if err != nil {
		return errorJSON(err.Error())
	}
	preparedCatalog = loadedCatalog
	preparedCatalogSet = true
	return `{"ok":true}`
}

func solvePreparedScenario(_ js.Value, args []js.Value) any {
	if len(args) < 1 || len(args) > 2 {
		return errorJSON("solvePreparedScenario expects a scenario JSON string and optional progress callback")
	}
	if args[0].Type() != js.TypeString {
		return errorJSON("solvePreparedScenario expects a scenario JSON string")
	}
	if !preparedCatalogSet {
		return errorJSON("catalog has not been installed")
	}
	var loadedScenario scenario.Scenario
	if err := json.Unmarshal([]byte(args[0].String()), &loadedScenario); err != nil {
		return errorJSON(err.Error())
	}
	result, err := websolve.SolvePreparedCatalog(preparedCatalog, loadedScenario, wasmOptions(args))
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(result.JSON)
}

func wasmOptions(args []js.Value) websolve.Options {
	var reporter solver.ProgressReporter
	if len(args) == 2 && args[1].Type() == js.TypeFunction {
		progressCallback := args[1]
		reporter = func(snapshot solver.ProgressSnapshot) {
			progressCallback.Invoke(progressToJS(snapshot))
		}
	}
	return websolve.Options{ProgressReporter: reporter, WorkerOverride: 1}
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
