package main

import (
	"github.com/Ledermayer/iaclens/internal/analyze"
	"testing"
)

func TestVerifyContracts(t *testing.T) {
	makeCase := func() (Report, Expectation) {
		var r Report
		r.Iaclens.Analysis = analyze.Result{CodebaseKind: "module", ComponentCounts: map[string]int{"module": 1}, ExampleCount: 1, Calls: []analyze.Call{{Path: ".", Stage: "checks"}}, Units: []analyze.Unit{{Path: "examples/default", Role: "example", Owner: ".", Classification: analyze.Classification{Kind: "deployment", Method: "signal"}, Checks: []analyze.CheckResult{{ID: "deliberate", Status: "fail"}}}}}
		e := Expectation{Kind: "module", Components: map[string]int{"module": 1}, Examples: 1, MinimumJevCalls: 1, Units: map[string]UnitExpectation{"examples/default": {Kind: "deployment", Method: "signal", Role: "example", Owner: "."}}, Checks: map[string]map[string]string{"examples/default": {"deliberate": "fail"}}}
		return r, e
	}
	t.Run("expected policy failure is successful evaluation", func(t *testing.T) {
		r, e := makeCase()
		if err := verify(r, e); err != nil {
			t.Fatal(err)
		}
	})
	cases := map[string]func(*Report){
		"missing live call":        func(r *Report) { r.Iaclens.Analysis.Calls = nil },
		"wrong ownership":          func(r *Report) { r.Iaclens.Analysis.Units[0].Owner = "other" },
		"unexpected policy status": func(r *Report) { r.Iaclens.Analysis.Units[0].Checks[0].Status = "pass" },
		"missing unit":             func(r *Report) { r.Iaclens.Analysis.Units = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r, e := makeCase()
			change(&r)
			if verify(r, e) == nil {
				t.Fatal("accepted incorrect result")
			}
		})
	}
}
