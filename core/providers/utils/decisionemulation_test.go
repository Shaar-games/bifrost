package utils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	schemas "github.com/maximhq/bifrost/core/schemas"
)

func mixedQuestions() map[string]schemas.DecisionQuestion {
	return map[string]schemas.DecisionQuestion{
		"is_frustrated": {Kind: schemas.DecisionKindNoul, Instructions: "Is the customer frustrated?"},
		"category": {
			Kind:         schemas.DecisionKindChoice,
			Instructions: "Pick the ticket category",
			Criteria:     map[string]interface{}{"billing": "money", "bug": "defects", "other": "else"},
		},
		"urgency": {
			Kind:         schemas.DecisionKindScore,
			Instructions: "How urgent?",
			Criteria:     []interface{}{"low", "medium", "high"},
		},
	}
}

func TestBuildDecisionSchemaShape(t *testing.T) {
	params, err := BuildDecisionSchema(mixedQuestions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Marshal and re-read so we assert on the emitted JSON schema.
	raw, err := sonic.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var schema map[string]interface{}
	if err := sonic.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("top type = %v", schema["type"])
	}
	props := schema["properties"].(map[string]interface{})
	for _, name := range []string{"is_frustrated", "category", "urgency"} {
		if _, ok := props[name]; !ok {
			t.Fatalf("missing property %q", name)
		}
	}

	noul := props["is_frustrated"].(map[string]interface{})["properties"].(map[string]interface{})
	if _, ok := noul["value"]; !ok {
		t.Error("noul missing value")
	}
	// A native noul answer is {type, noul} only - the value near 0.5 is the
	// uncertainty signal - so confidence stays an optional extra the model may
	// volunteer, never a required field.
	if _, ok := noul["confidence"]; !ok {
		t.Error("noul confidence should remain available as an optional field")
	}
	noulRequired := props["is_frustrated"].(map[string]interface{})["required"].([]interface{})
	if len(noulRequired) != 1 || noulRequired[0] != "value" {
		t.Errorf("noul required = %v, want [value] only", noulRequired)
	}

	choice := props["category"].(map[string]interface{})["properties"].(map[string]interface{})
	enum := choice["choice"].(map[string]interface{})["enum"].([]interface{})
	if len(enum) != 3 {
		t.Errorf("choice enum = %v", enum)
	}
	if _, ok := choice["probabilities"]; !ok {
		t.Error("choice missing probabilities")
	}
	// Probabilities are a required field on native choice and score answers,
	// so the tool schema must require them from the emulating model too.
	choiceRequired := props["category"].(map[string]interface{})["required"].([]interface{})
	if !containsAnyString(choiceRequired, "probabilities") {
		t.Errorf("choice required = %v, want probabilities included", choiceRequired)
	}

	score := props["urgency"].(map[string]interface{})["properties"].(map[string]interface{})
	if !strings.Contains(score["value"].(map[string]interface{})["description"].(string), "high") {
		t.Error("score value description should mention the levels")
	}
	scoreRequired := props["urgency"].(map[string]interface{})["required"].([]interface{})
	if !containsAnyString(scoreRequired, "probabilities") {
		t.Errorf("score required = %v, want probabilities included", scoreRequired)
	}
}

// containsAnyString reports whether a decoded []interface{} contains s.
func containsAnyString(list []interface{}, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// structuredQuestions carries the spec's richer criteria shapes: object and
// null choice descriptions, and object levels in a score array.
func structuredQuestions() map[string]schemas.DecisionQuestion {
	return map[string]schemas.DecisionQuestion{
		"approve": {
			Kind:         schemas.DecisionKindNoul,
			Instructions: "Approve the refund?",
			Criteria: map[string]any{
				"true":  map[string]any{"meaning": "refund it"},
				"false": []any{"deny it", "no evidence"},
			},
		},
		"category": {
			Kind:         schemas.DecisionKindChoice,
			Instructions: "Pick the ticket category",
			Criteria: map[string]any{
				"billing": map[string]any{"rubric": "money issues"},
				"bug":     []any{"crash", "defect"},
				"support": "service questions",
				"other":   nil,
			},
		},
		"urgency": {
			Kind:         schemas.DecisionKindScore,
			Instructions: "How urgent?",
			Criteria: []any{
				"low",
				map[string]any{"examples": []any{"outage"}, "level": "high"},
				[]any{"critical", "churn risk"},
			},
		},
	}
}

func TestBuildDecisionSchemaStructuredCriteria(t *testing.T) {
	params, err := BuildDecisionSchema(structuredQuestions())
	if err != nil {
		t.Fatalf("structured criteria rejected: %v", err)
	}
	raw, err := sonic.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var schema map[string]any
	if err := sonic.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	props := schema["properties"].(map[string]any)

	noulDesc := props["approve"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)["description"].(string)
	// The true/false rubric descriptions must reach the model: objects and
	// arrays render as sorted JSON.
	if !strings.Contains(noulDesc, `true={"meaning":"refund it"}`) || !strings.Contains(noulDesc, `false=["deny it","no evidence"]`) {
		t.Errorf("noul description missing true/false criteria: %q", noulDesc)
	}

	choice := props["category"].(map[string]any)["properties"].(map[string]any)["choice"].(map[string]any)
	enum := choice["enum"].([]any)
	if len(enum) != 4 {
		t.Fatalf("choice enum should keep all 4 options including the null-described one: %v", enum)
	}
	choiceDesc := choice["description"].(string)
	// Option rubric descriptions must reach the model: strings verbatim,
	// objects and arrays as JSON, null options simply skipped.
	for _, want := range []string{`{"rubric":"money issues"}`, `["crash","defect"]`, "service questions"} {
		if !strings.Contains(choiceDesc, want) {
			t.Errorf("choice description missing option rubric %q: %q", want, choiceDesc)
		}
	}

	scoreDesc := props["urgency"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)["description"].(string)
	for _, want := range []string{"0=low", `{"examples":["outage"],"level":"high"}`, `["critical","churn risk"]`} {
		if !strings.Contains(scoreDesc, want) {
			t.Errorf("score description missing level %q: %q", want, scoreDesc)
		}
	}
	if strings.Contains(scoreDesc, "map[") {
		t.Errorf("score description leaks Go map syntax: %q", scoreDesc)
	}
}

func TestParseDecisionAnswersStructuredScoreLegend(t *testing.T) {
	args := `{
		"approve": {"value": 0.9, "confidence": 0.8},
		"category": {"choice": "other", "confidence": 0.7, "probabilities": {"billing": 0.1, "bug": 0.1, "support": 0.1, "other": 0.7}},
		"urgency": {"value": 1, "confidence": 0.6, "probabilities": {"0": 0.1, "1": 0.8, "2": 0.1}}
	}`
	answers, err := ParseDecisionAnswers([]byte(args), structuredQuestions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answers["category"].Value.(string) != "other" {
		t.Errorf("null-described choice option rejected: %+v", answers["category"])
	}
	// The legend echoes each level's description verbatim, matching the native
	// API's shape (values are strings, objects, or arrays - not stringified).
	legend := answers["urgency"].Legend
	if legend["0"] != "low" {
		t.Errorf("string legend level should pass through verbatim, got %#v", legend["0"])
	}
	if !reflect.DeepEqual(legend["1"], map[string]any{"examples": []any{"outage"}, "level": "high"}) {
		t.Errorf("object legend level should echo verbatim, got %#v", legend["1"])
	}
	if !reflect.DeepEqual(legend["2"], []any{"critical", "churn risk"}) {
		t.Errorf("array legend level should echo verbatim, got %#v", legend["2"])
	}
}

// TestBuildDecisionSchemaTypedStringMapCriteria covers the map[string]string
// input branch Go SDK callers use for noul and choice criteria.
func TestBuildDecisionSchemaTypedStringMapCriteria(t *testing.T) {
	questions := map[string]schemas.DecisionQuestion{
		"approve": {
			Kind:         schemas.DecisionKindNoul,
			Instructions: "Approve?",
			Criteria:     map[string]string{"true": "grant it", "false": "deny it"},
		},
		"bucket": {
			Kind:         schemas.DecisionKindChoice,
			Instructions: "Bucket?",
			Criteria:     map[string]string{"a": "first bucket", "b": "second bucket"},
		},
	}
	params, err := BuildDecisionSchema(questions)
	if err != nil {
		t.Fatalf("typed string map criteria rejected: %v", err)
	}
	raw, err := sonic.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var schema map[string]any
	if err := sonic.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	props := schema["properties"].(map[string]any)

	noulDesc := props["approve"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)["description"].(string)
	if !strings.Contains(noulDesc, "true=grant it") || !strings.Contains(noulDesc, "false=deny it") {
		t.Errorf("noul description missing typed-map criteria: %q", noulDesc)
	}
	choiceDesc := props["bucket"].(map[string]any)["properties"].(map[string]any)["choice"].(map[string]any)["description"].(string)
	if !strings.Contains(choiceDesc, "a=first bucket") || !strings.Contains(choiceDesc, "b=second bucket") {
		t.Errorf("choice description missing typed-map rubrics: %q", choiceDesc)
	}
}

func TestBuildDecisionSchemaEmpty(t *testing.T) {
	if _, err := BuildDecisionSchema(map[string]schemas.DecisionQuestion{}); err == nil {
		t.Fatal("expected error for empty questions")
	}
}

func TestParseDecisionAnswersValid(t *testing.T) {
	args := `{
		"is_frustrated": {"value": 0.9, "confidence": 0.8},
		"category": {"choice": "billing", "confidence": 0.7, "probabilities": {"billing": 0.7, "bug": 0.2, "other": 0.1}},
		"urgency": {"value": 2, "confidence": 0.6, "probabilities": {"0": 0.1, "1": 0.1, "2": 0.8}}
	}`
	answers, err := ParseDecisionAnswers([]byte(args), mixedQuestions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answers["is_frustrated"].Kind != schemas.DecisionKindNoul || answers["is_frustrated"].Value.(float64) != 0.9 {
		t.Errorf("noul = %+v", answers["is_frustrated"])
	}
	if *answers["is_frustrated"].Confidence != 0.8 {
		t.Errorf("noul confidence = %v", answers["is_frustrated"].Confidence)
	}
	if answers["category"].Value.(string) != "billing" {
		t.Errorf("choice = %+v", answers["category"])
	}
	if answers["category"].Probabilities["billing"] != 0.7 {
		t.Errorf("choice probabilities = %+v", answers["category"].Probabilities)
	}
	if answers["urgency"].Value.(float64) != 2 {
		t.Errorf("score = %+v", answers["urgency"])
	}
	if answers["urgency"].Legend["2"] != "high" {
		t.Errorf("score legend = %+v", answers["urgency"].Legend)
	}
}

func TestParseDecisionAnswersNoulConfidenceOptional(t *testing.T) {
	// The schema no longer requires confidence on noul (the native answer is
	// {type, noul} only), so a model omitting it must parse fine; one it
	// volunteers is still kept and validated.
	args := `{
		"is_frustrated": {"value": 0.9},
		"category": {"choice": "billing", "confidence": 0.7, "probabilities": {"billing": 0.8, "bug": 0.1, "other": 0.1}},
		"urgency": {"value": 1, "confidence": 0.5, "probabilities": {"0": 0.1, "1": 0.8, "2": 0.1}}
	}`
	answers, err := ParseDecisionAnswers([]byte(args), mixedQuestions())
	if err != nil {
		t.Fatalf("noul answer without confidence rejected: %v", err)
	}
	if answers["is_frustrated"].Confidence != nil {
		t.Errorf("omitted noul confidence should stay nil, got %v", *answers["is_frustrated"].Confidence)
	}

	// A volunteered but invalid noul confidence is still rejected.
	bad := `{
		"is_frustrated": {"value": 0.9, "confidence": 1.4},
		"category": {"choice": "billing", "confidence": 0.7, "probabilities": {"billing": 0.8, "bug": 0.1, "other": 0.1}},
		"urgency": {"value": 1, "confidence": 0.5, "probabilities": {"0": 0.1, "1": 0.8, "2": 0.1}}
	}`
	if _, err := ParseDecisionAnswers([]byte(bad), mixedQuestions()); err == nil {
		t.Fatal("volunteered out-of-range noul confidence must be rejected")
	}
}

func TestParseDecisionAnswersRequiresProbabilities(t *testing.T) {
	// Probabilities are required on native choice and score answers, so an
	// emulated answer omitting them must be rejected - a response without them
	// would fail strict SDK clients - while noul needs none.
	missingChoice := `{
		"is_frustrated": {"value": 0.9},
		"category": {"choice": "billing", "confidence": 0.7},
		"urgency": {"value": 1, "confidence": 0.5, "probabilities": {"0": 0.1, "1": 0.8, "2": 0.1}}
	}`
	if _, err := ParseDecisionAnswers([]byte(missingChoice), mixedQuestions()); err == nil || !strings.Contains(err.Error(), "probabilities") {
		t.Fatalf("choice answer without probabilities must be rejected, got %v", err)
	}

	missingScore := `{
		"is_frustrated": {"value": 0.9},
		"category": {"choice": "billing", "confidence": 0.7, "probabilities": {"billing": 0.8, "bug": 0.1, "other": 0.1}},
		"urgency": {"value": 1, "confidence": 0.5}
	}`
	if _, err := ParseDecisionAnswers([]byte(missingScore), mixedQuestions()); err == nil || !strings.Contains(err.Error(), "probabilities") {
		t.Fatalf("score answer without probabilities must be rejected, got %v", err)
	}

	// A partial distribution that happens to sum to 1 is still incomplete:
	// the contract is a probability for EVERY option and level.
	partialChoice := `{
		"is_frustrated": {"value": 0.9},
		"category": {"choice": "billing", "confidence": 0.7, "probabilities": {"billing": 1}},
		"urgency": {"value": 1, "confidence": 0.5, "probabilities": {"0": 0.1, "1": 0.8, "2": 0.1}}
	}`
	if _, err := ParseDecisionAnswers([]byte(partialChoice), mixedQuestions()); err == nil || !strings.Contains(err.Error(), "probabilities") {
		t.Fatalf("one-hot partial choice distribution must be rejected, got %v", err)
	}

	partialScore := `{
		"is_frustrated": {"value": 0.9},
		"category": {"choice": "billing", "confidence": 0.7, "probabilities": {"billing": 0.8, "bug": 0.1, "other": 0.1}},
		"urgency": {"value": 1, "confidence": 0.5, "probabilities": {"1": 1}}
	}`
	if _, err := ParseDecisionAnswers([]byte(partialScore), mixedQuestions()); err == nil || !strings.Contains(err.Error(), "probabilities") {
		t.Fatalf("partial score distribution must be rejected, got %v", err)
	}

	complete := `{
		"is_frustrated": {"value": 0.9},
		"category": {"choice": "billing", "confidence": 0.7, "probabilities": {"billing": 0.8, "bug": 0.1, "other": 0.1}},
		"urgency": {"value": 1, "confidence": 0.5, "probabilities": {"0": 0.1, "1": 0.8, "2": 0.1}}
	}`
	answers, err := ParseDecisionAnswers([]byte(complete), mixedQuestions())
	if err != nil {
		t.Fatalf("complete answers rejected: %v", err)
	}
	if answers["urgency"].Probabilities["1"] != 0.8 {
		t.Errorf("score probabilities lost: %+v", answers["urgency"].Probabilities)
	}
}

func TestParseDecisionAnswersRejections(t *testing.T) {
	q := mixedQuestions()
	cases := map[string]string{
		"noul out of range": `{"is_frustrated":{"value":1.4,"confidence":0.5},"category":{"choice":"billing","confidence":0.5},"urgency":{"value":1,"confidence":0.5}}`,
		"unknown choice":    `{"is_frustrated":{"value":0.5,"confidence":0.5},"category":{"choice":"nope","confidence":0.5},"urgency":{"value":1,"confidence":0.5}}`,
		"missing question":  `{"is_frustrated":{"value":0.5,"confidence":0.5},"category":{"choice":"billing","confidence":0.5}}`,
		"noul not a number": `{"is_frustrated":{"value":"high","confidence":0.5},"category":{"choice":"billing","confidence":0.5},"urgency":{"value":1,"confidence":0.5}}`,
		"malformed json":    `not json`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDecisionAnswers([]byte(args), q); err == nil {
				t.Fatalf("expected rejection for %q", name)
			}
		})
	}
}

func TestBuildDecisionToolName(t *testing.T) {
	tool, err := BuildDecisionResponsesTool(mixedQuestions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.Type != schemas.ResponsesToolTypeFunction {
		t.Errorf("tool type = %v", tool.Type)
	}
	if tool.Name == nil || *tool.Name != DecisionToolName {
		t.Errorf("tool name = %v", tool.Name)
	}
	if tool.ResponsesToolFunction == nil || tool.ResponsesToolFunction.Parameters == nil {
		t.Errorf("tool function params missing: %+v", tool)
	}
}
