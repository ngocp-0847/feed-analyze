package research

import "testing"

func TestVizNormalize(t *testing.T) {
	v := &DeepViz{
		CausalNodes:  []VizNode{{ID: "a"}, {ID: "b"}},
		CausalEdges:  []VizEdge{{From: "a", To: "b", Strength: 9}, {From: "a", To: "x"}, {From: "a", To: "a"}},
		Stakeholders: []VizActor{{Stance: 7, Influence: 0}},
		Scenarios:    []VizScenario{{Probability: 140}},
		VNImpacts:    []VizImpact{{Score: -5}},
	}
	v.normalize()
	if len(v.CausalEdges) != 1 || v.CausalEdges[0].Strength != 3 {
		t.Fatalf("edges=%+v", v.CausalEdges)
	}
	if v.Stakeholders[0].Stance != 2 || v.Stakeholders[0].Influence != 1 || v.Scenarios[0].Probability != 100 || v.VNImpacts[0].Score != -2 {
		t.Fatalf("clamp sai: %+v %+v %+v", v.Stakeholders, v.Scenarios, v.VNImpacts)
	}
	if VizKind("deep:abc") != "deepviz:abc" {
		t.Fatal("VizKind")
	}
}
