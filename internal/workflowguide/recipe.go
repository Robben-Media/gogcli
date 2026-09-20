package workflowguide

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	FactCaller = "caller"
	FactLookup = "lookup"

	StatusReady         = "ready"
	StatusBlocked       = "blocked"
	StatusUnavailable   = "unavailable"
	StatusNotApplicable = "not_applicable"

	FactsCallerSupplied = "caller_supplied"

	KindLookup   = "lookup"
	KindWrite    = "write"
	KindReadback = "readback"

	MaxRecipeSteps         = 8
	MaxRecipeInputs        = 12
	MaxKnownFactKeys       = 16
	MaxKnownFactKeyRunes   = 64
	MaxKnownFactValueRunes = 256
)

var (
	ErrUnknownIntent  = errors.New("unknown intent_id")
	ErrIntentRequired = errors.New("intent_id is required")
)

// FactDef is one caller input or lookup output used by a recipe.
type FactDef struct {
	Key          string            `json:"key"`
	Format       string            `json:"format"`
	Reason       string            `json:"reason"`
	Source       string            `json:"source"`
	ProducedBy   string            `json:"produced_by,omitempty"`
	When         map[string]string `json:"when,omitempty"`
	Optional     bool              `json:"optional,omitempty"`
	OptionalWhen map[string]string `json:"optional_when,omitempty"`
}

// InputDef is one native request field.
type InputDef struct {
	Name     string `json:"name"`
	Location string `json:"location"`
	Format   string `json:"format"`
	Source   string `json:"source"`
}

// RecipeStep is one linear lookup, write, or readback.
type RecipeStep struct {
	Operation   string            `json:"operation"`
	Kind        string            `json:"kind"`
	SkipIfFacts []string          `json:"skip_if_facts,omitempty"`
	OnlyIf      map[string]string `json:"only_if,omitempty"`
	OnlyIfFacts []string          `json:"only_if_facts,omitempty"`
	Produces    []string          `json:"produces,omitempty"`
	Inputs      []InputDef        `json:"inputs,omitempty"`
}

// StopRules tell the caller when not to continue or replay.
type StopRules struct {
	Revision    string `json:"revision"`
	Idempotency string `json:"idempotency"`
	Reconcile   string `json:"reconcile"`
}

// Recipe is a static, reviewed preparation plan for one intent.
type Recipe struct {
	IntentID        string       `json:"intent_id"`
	Service         string       `json:"service"`
	AppliesTo       []string     `json:"applies_to"`
	RequiredFacts   []FactDef    `json:"required_facts"`
	Steps           []RecipeStep `json:"steps"`
	CallEstimateMin int          `json:"call_estimate_min"`
	CallEstimateMax int          `json:"call_estimate_max"`
	StopRules       StopRules    `json:"stop_rules"`
	Gap             string       `json:"gap,omitempty"`
}

// MissingFact is a caller-supplied input that is still required.
type MissingFact struct {
	Key    string `json:"key"`
	Format string `json:"format"`
	Reason string `json:"reason"`
	Source string `json:"source,omitempty"`
}

// UnresolvedOutput is a lookup result still to be obtained at execution.
type UnresolvedOutput struct {
	Key        string `json:"key"`
	Format     string `json:"format"`
	ProducedBy string `json:"produced_by"`
	Reason     string `json:"reason"`
}

// PreparedInput is one field on an executable step.
type PreparedInput struct {
	Name     string `json:"name"`
	Location string `json:"location"`
	Format   string `json:"format"`
	Source   string `json:"source"`
}

// PreparedStep is a recipe step after skip filtering.
type PreparedStep struct {
	Operation      string          `json:"operation"`
	Kind           string          `json:"kind"`
	Availability   string          `json:"availability,omitempty"`
	RequiredInputs []PreparedInput `json:"required_inputs,omitempty"`
}

// CallEstimate counts planned provider calls excluding retries.
type CallEstimate struct {
	Min             int  `json:"min"`
	Max             int  `json:"max"`
	ExcludesRetries bool `json:"excludes_retries"`
}

// PreparedRecipe is the optional capabilities_describe preparation payload.
type PreparedRecipe struct {
	IntentID             string             `json:"intent_id"`
	Service              string             `json:"service"`
	Status               string             `json:"status"`
	FactsStatus          string             `json:"facts_status"`
	SuppliedFactKeys     []string           `json:"supplied_fact_keys,omitempty"`
	MissingPrerequisites []MissingFact      `json:"missing_prerequisites,omitempty"`
	UnresolvedOutputs    []UnresolvedOutput `json:"unresolved_outputs,omitempty"`
	Steps                []PreparedStep     `json:"steps,omitempty"`
	CallEstimate         CallEstimate       `json:"call_estimate"`
	StopRules            StopRules          `json:"stop_rules"`
	Gap                  string             `json:"gap,omitempty"`
}

//go:embed recipes.json
var recipeData []byte

var recipesByIntent = loadRecipes()

func loadRecipes() map[string]Recipe {
	var entries []Recipe
	if err := json.Unmarshal(recipeData, &entries); err != nil {
		panic(err)
	}

	out := make(map[string]Recipe, len(entries))
	for _, entry := range entries {
		if entry.IntentID == "" {
			panic("workflowguide: recipe missing intent_id")
		}

		if _, exists := out[entry.IntentID]; exists {
			panic("workflowguide: duplicate recipe " + entry.IntentID)
		}

		if len(entry.Steps) > MaxRecipeSteps {
			panic("workflowguide: recipe " + entry.IntentID + " exceeds step bound")
		}
		out[entry.IntentID] = entry
	}

	return out
}

// LookupRecipe returns the static recipe for an explicit intent_id.
func LookupRecipe(intentID string) (Recipe, bool) {
	recipe, ok := recipesByIntent[strings.TrimSpace(intentID)]
	if !ok {
		return Recipe{}, false
	}

	return cloneRecipe(recipe), true
}

// Prepare builds one local recipe view. It performs no network requests and
// does not treat known_facts as verified provider state.
func Prepare(intentID, selectedOperation string, facts map[string]string) (PreparedRecipe, error) {
	intentID = strings.TrimSpace(intentID)
	if intentID == "" {
		return PreparedRecipe{}, ErrIntentRequired
	}

	recipe, ok := LookupRecipe(intentID)
	if !ok {
		return PreparedRecipe{}, ErrUnknownIntent
	}

	selectedOperation = strings.TrimSpace(selectedOperation)
	if !appliesTo(recipe, selectedOperation) {
		return PreparedRecipe{
			IntentID:     recipe.IntentID,
			Service:      recipe.Service,
			Status:       StatusNotApplicable,
			FactsStatus:  FactsCallerSupplied,
			Gap:          recipe.Gap,
			CallEstimate: CallEstimate{ExcludesRetries: true},
			StopRules:    recipe.StopRules,
		}, nil
	}

	facts = sanitizeFacts(facts)
	steps := selectSteps(recipe, facts)
	missing := missingCallerFacts(recipe, facts)
	unresolved := unresolvedLookupOutputs(recipe, steps, facts)
	supplied := suppliedKeys(recipe, facts)

	calls := len(steps)

	status := StatusReady
	if recipe.Gap != "" && len(steps) == 0 {
		status = StatusUnavailable
	} else if len(missing) > 0 {
		status = StatusBlocked
	}

	return PreparedRecipe{
		IntentID:             recipe.IntentID,
		Service:              recipe.Service,
		Status:               status,
		FactsStatus:          FactsCallerSupplied,
		SuppliedFactKeys:     supplied,
		MissingPrerequisites: missing,
		UnresolvedOutputs:    unresolved,
		Steps:                steps,
		CallEstimate: CallEstimate{
			Min:             calls,
			Max:             calls,
			ExcludesRetries: true,
		},
		StopRules: recipe.StopRules,
		Gap:       recipe.Gap,
	}, nil
}

func appliesTo(recipe Recipe, selected string) bool {
	if selected == "" {
		return false
	}

	for _, name := range recipe.AppliesTo {
		if name == selected {
			return true
		}
	}

	return false
}

func sanitizeFacts(facts map[string]string) map[string]string {
	if len(facts) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(facts))
	for key, value := range facts {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
	}

	return out
}

func hasFact(facts map[string]string, key string) bool {
	value, ok := facts[key]
	return ok && strings.TrimSpace(value) != ""
}

func selectSteps(recipe Recipe, facts map[string]string) []PreparedStep {
	out := make([]PreparedStep, 0, len(recipe.Steps))
	for _, step := range recipe.Steps {
		if !onlyIfMatches(step, facts) {
			continue
		}

		if allFactsPresent(facts, step.SkipIfFacts) {
			continue
		}

		inputs := make([]PreparedInput, 0, len(step.Inputs))
		for i, input := range step.Inputs {
			if i >= MaxRecipeInputs {
				break
			}

			inputs = append(inputs, PreparedInput{
				Name:     input.Name,
				Location: input.Location,
				Format:   input.Format,
				Source:   inputSource(recipe, input.Source, facts),
			})
		}

		out = append(out, PreparedStep{
			Operation:      step.Operation,
			Kind:           step.Kind,
			RequiredInputs: inputs,
		})
		if len(out) == MaxRecipeSteps {
			break
		}
	}

	return out
}

func onlyIfMatches(step RecipeStep, facts map[string]string) bool {
	for key, want := range step.OnlyIf {
		if facts[key] != want {
			return false
		}
	}

	for _, key := range step.OnlyIfFacts {
		if !hasFact(facts, key) {
			return false
		}
	}

	return true
}

func allFactsPresent(facts map[string]string, keys []string) bool {
	if len(keys) == 0 {
		return false
	}

	for _, key := range keys {
		if !hasFact(facts, key) {
			return false
		}
	}

	return true
}

func missingCallerFacts(recipe Recipe, facts map[string]string) []MissingFact {
	var missing []MissingFact

	for _, fact := range recipe.RequiredFacts {
		if fact.Source == FactLookup {
			continue
		}

		if fact.Optional {
			continue
		}

		if !whenMatches(fact.When, facts) {
			continue
		}

		if len(fact.OptionalWhen) > 0 && whenMatches(fact.OptionalWhen, facts) {
			continue
		}

		if hasFact(facts, fact.Key) {
			continue
		}
		missing = append(missing, MissingFact{
			Key:    fact.Key,
			Format: fact.Format,
			Reason: fact.Reason,
			Source: FactCaller,
		})
	}

	return missing
}

func unresolvedLookupOutputs(recipe Recipe, steps []PreparedStep, facts map[string]string) []UnresolvedOutput {
	planned := make(map[string]RecipeStep, len(steps))
	for _, step := range steps {
		for _, raw := range recipe.Steps {
			if raw.Operation == step.Operation && raw.Kind == step.Kind {
				planned[step.Operation+"/"+step.Kind] = raw
				break
			}
		}
	}

	var out []UnresolvedOutput
	seen := map[string]struct{}{}

	for _, fact := range recipe.RequiredFacts {
		if fact.Source != FactLookup {
			continue
		}

		if hasFact(facts, fact.Key) {
			continue
		}
		producedBy := fact.ProducedBy
		stillPlanned := false

		for _, step := range steps {
			if step.Kind != KindLookup {
				continue
			}

			raw := planned[step.Operation+"/"+step.Kind]
			if produces(raw, fact.Key) || raw.Operation == producedBy {
				stillPlanned = true

				if producedBy == "" {
					producedBy = step.Operation
				}

				break
			}
		}

		if !stillPlanned {
			continue
		}

		if _, ok := seen[fact.Key]; ok {
			continue
		}
		seen[fact.Key] = struct{}{}
		out = append(out, UnresolvedOutput{
			Key:        fact.Key,
			Format:     fact.Format,
			ProducedBy: producedBy,
			Reason:     fact.Reason,
		})
	}

	return out
}

func produces(step RecipeStep, key string) bool {
	for _, name := range step.Produces {
		if name == key {
			return true
		}
	}

	return false
}

func whenMatches(when map[string]string, facts map[string]string) bool {
	if len(when) == 0 {
		return true
	}

	for key, want := range when {
		if facts[key] != want {
			return false
		}
	}

	return true
}

func suppliedKeys(recipe Recipe, facts map[string]string) []string {
	seen := map[string]struct{}{}
	var out []string

	add := func(key string) {
		if !hasFact(facts, key) {
			return
		}

		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	for _, fact := range recipe.RequiredFacts {
		add(fact.Key)

		for key := range fact.OptionalWhen {
			add(key)
		}

		for key := range fact.When {
			add(key)
		}
	}

	for _, step := range recipe.Steps {
		for _, key := range step.SkipIfFacts {
			add(key)
		}

		for _, key := range step.Produces {
			add(key)
		}
	}

	return out
}

func cloneRecipe(recipe Recipe) Recipe {
	recipe.AppliesTo = append([]string(nil), recipe.AppliesTo...)

	recipe.RequiredFacts = append([]FactDef(nil), recipe.RequiredFacts...)
	for i := range recipe.RequiredFacts {
		recipe.RequiredFacts[i].When = cloneStringMap(recipe.RequiredFacts[i].When)
		recipe.RequiredFacts[i].OptionalWhen = cloneStringMap(recipe.RequiredFacts[i].OptionalWhen)
	}

	recipe.Steps = append([]RecipeStep(nil), recipe.Steps...)
	for i := range recipe.Steps {
		recipe.Steps[i].SkipIfFacts = append([]string(nil), recipe.Steps[i].SkipIfFacts...)
		recipe.Steps[i].OnlyIfFacts = append([]string(nil), recipe.Steps[i].OnlyIfFacts...)
		recipe.Steps[i].Produces = append([]string(nil), recipe.Steps[i].Produces...)
		recipe.Steps[i].Inputs = append([]InputDef(nil), recipe.Steps[i].Inputs...)
		recipe.Steps[i].OnlyIf = cloneStringMap(recipe.Steps[i].OnlyIf)
	}

	return recipe
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}

	return out
}

// ValidateKnownFacts returns a reason if the caller-supplied map exceeds bounds.
func ValidateKnownFacts(facts map[string]string) string {
	if len(facts) == 0 {
		return ""
	}

	if len(facts) > MaxKnownFactKeys {
		return "known_facts exceeds the 16 entry limit"
	}

	for key, value := range facts {
		if utf8.RuneCountInString(key) > MaxKnownFactKeyRunes {
			return "known_facts key exceeds the 64 character limit"
		}

		if utf8.RuneCountInString(value) > MaxKnownFactValueRunes {
			return "known_facts value exceeds the 256 character limit"
		}
	}

	return ""
}

func inputSource(recipe Recipe, source string, facts map[string]string) string {
	key, ok := strings.CutPrefix(source, "fact:")
	if !ok || hasFact(facts, key) {
		return source
	}

	for _, fact := range recipe.RequiredFacts {
		if fact.Key == key && fact.Source == FactLookup {
			return "previous_step:" + key
		}
	}

	return source
}
