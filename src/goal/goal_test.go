package goal

import (
	"strings"
	"testing"
)

func valid() Goal {
	return Goal{Title: "Trip to Japan", Code: "TRIP", Type: Save, Currency: "BRL", Target: 1000000}
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid goal: %v", err)
	}
	withDate := valid()
	withDate.DueDate = "2027-06-30"
	if err := withDate.Validate(); err != nil {
		t.Fatalf("goal with a final date: %v", err)
	}
	for name, tc := range map[string]struct {
		change func(*Goal)
		want   string
	}{
		"no title":     {func(g *Goal) { g.Title = "  " }, "title is required"},
		"long title":   {func(g *Goal) { g.Title = strings.Repeat("a", MaxTitle+1) }, "title is too long"},
		"long text":    {func(g *Goal) { g.Description = strings.Repeat("a", MaxDescription+1) }, "description is too long"},
		"bad code":     {func(g *Goal) { g.Code = "A B" }, "code"},
		"bad type":     {func(g *Goal) { g.Type = "budget" }, `unknown type "budget": use one of save, payoff, purchase`},
		"bad currency": {func(g *Goal) { g.Currency = "XYZ" }, "unknown currency"},
		"zero target":  {func(g *Goal) { g.Target = 0 }, "target must be more than zero"},
		"bad date":     {func(g *Goal) { g.DueDate = "30/06/2027" }, "invalid final date"},
	} {
		goal := valid()
		tc.change(&goal)
		if err := goal.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", name, err, tc.want)
		}
	}
}

func TestProgress(t *testing.T) {
	for _, tc := range []struct {
		saved, target, progress, remaining int64
		percent                            int
	}{
		{0, 1000, 0, 1000, 0},
		{250, 1000, 250, 750, 25},
		{999, 1000, 999, 1, 99},
		{1000, 1000, 1000, 0, 100},
		{1500, 1000, 1500, 0, 100},
		{-300, 1000, 0, 1000, 0},
		{1, 3, 1, 2, 33},
	} {
		goal := Goal{Saved: tc.saved, Target: tc.target}
		if goal.Progress() != tc.progress || goal.Remaining() != tc.remaining || goal.Percent() != tc.percent {
			t.Errorf("saved %d of %d: progress %d, remaining %d, percent %d; want %d, %d, %d",
				tc.saved, tc.target, goal.Progress(), goal.Remaining(), goal.Percent(), tc.progress, tc.remaining, tc.percent)
		}
	}
}

func TestStatus(t *testing.T) {
	const today = "2026-11-17"
	for _, tc := range []struct {
		saved int64
		due   string
		want  string
	}{
		{0, "", StatusActive},
		{500, "2026-12-31", StatusActive},
		{500, "2026-11-17", StatusActive},
		{500, "2026-11-16", StatusOverdue},
		{1000, "2026-11-16", StatusReached},
		{1000, "", StatusReached},
		{2000, "2020-01-01", StatusReached},
	} {
		goal := Goal{Saved: tc.saved, Target: 1000, DueDate: tc.due}
		if got := goal.Status(today); got != tc.want {
			t.Errorf("saved %d, due %q: status %s, want %s", tc.saved, tc.due, got, tc.want)
		}
	}
}

func TestTypes(t *testing.T) {
	if got := strings.Join(Types(), ","); got != "save,payoff,purchase" {
		t.Fatalf("Types() = %s", got)
	}
	if !ValidType(PayOff) || ValidType("") || ValidType("Save") {
		t.Fatal("ValidType is wrong")
	}
}

func TestEffect(t *testing.T) {
	for _, tc := range []struct {
		goal, kind string
		amount     int64
		want       int64
	}{
		{Save, "income", 500, 500},
		{Save, "expense", -200, -200},
		{Save, "transfer", -300, 300},
		{Save, "transfer", 300, 0},
		{Purchase, "expense", -200, -200},
		{PayOff, "expense", -200, 200},
		{PayOff, "income", 500, -500},
		{PayOff, "transfer", -300, 300},
		{PayOff, "transfer", 300, 0},
		{Save, "payment", -100, 0},
	} {
		if got := (Goal{Type: tc.goal}).Effect(tc.kind, tc.amount); got != tc.want {
			t.Errorf("%s goal, %s of %d: effect %d, want %d", tc.goal, tc.kind, tc.amount, got, tc.want)
		}
	}
}
