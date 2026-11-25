package transaction

import (
	"strings"
	"testing"
)

func TestParseDate(t *testing.T) {
	for input, want := range map[string]string{
		"17/11/2026":   "2026-11-17",
		"2026/11/17":   "2026-11-17",
		"2026-11-17":   "2026-11-17",
		" 1/2/2026 ":   "2026-02-01",
		"2026/2/1":     "2026-02-01",
		"29/02/2028":   "2028-02-29",
		"31/12/1999":   "1999-12-31",
		"2026-01-05\n": "2026-01-05",
	} {
		if got, err := ParseDate(input); err != nil || got != want {
			t.Errorf("ParseDate(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "17/11", "2026", "31/02/2026", "11/17/2026", "17-11-2026", "tomorrow", "2026/13/01"} {
		if got, err := ParseDate(input); err == nil {
			t.Errorf("ParseDate(%q) = %q, want error", input, got)
		}
	}
	if got := FormatDate("2026-11-17"); got != "17/11/2026" {
		t.Fatalf("FormatDate = %q", got)
	}
	if got := FormatDate("bad"); got != "bad" {
		t.Fatalf("FormatDate of invalid text = %q", got)
	}
}

func TestParsePeriod(t *testing.T) {
	for input, want := range map[string]string{
		"2026":       "2026",
		"11/2026":    "2026-11",
		"2026/11":    "2026-11",
		"2026-11":    "2026-11",
		"2/2026":     "2026-02",
		"17/11/2026": "2026-11-17",
		"2026/11/17": "2026-11-17",
	} {
		if got, err := ParsePeriod(input); err != nil || got != want {
			t.Errorf("ParsePeriod(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "26", "13/2026", "2026/13", "november", "17/11"} {
		if got, err := ParsePeriod(input); err == nil {
			t.Errorf("ParsePeriod(%q) = %q, want error", input, got)
		}
	}
}

func TestParseTags(t *testing.T) {
	for input, want := range map[string]string{
		"":                            "",
		"#market":                     "#market",
		"market":                      "#market",
		"#Market  #WEEKLY":            "#market #weekly",
		"#a, #b,c":                    "#a #b #c",
		"#another_thing #x1":          "#another_thing #x1",
		"#dup #DUP #other":            "#dup #other",
		"#alimentação":                "#alimentação",
		"  #spaces   around  ":        "#spaces #around",
		"#b #a":                       "#b #a",
		"#" + strings.Repeat("a", 30): "#" + strings.Repeat("a", 30),
	} {
		if got, err := ParseTags(input); err != nil || got != want {
			t.Errorf("ParseTags(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"#", "#a-b", "#a.b", "##a", "#a#b", "#" + strings.Repeat("a", 31), "#ok #bad!"} {
		if got, err := ParseTags(input); err == nil {
			t.Errorf("ParseTags(%q) = %q, want error", input, got)
		}
	}
}

func TestAccepts(t *testing.T) {
	for _, tc := range []struct {
		kind, color string
		want        bool
	}{
		{Expense, "red", true}, {Expense, "grey", true}, {Expense, "green", false},
		{Income, "green", true}, {Income, "grey", true}, {Income, "red", false},
		{Transfer, "grey", true}, {Transfer, "red", false}, {Transfer, "green", false},
		{"other", "grey", false}, {Expense, "blue", false},
	} {
		if got := Accepts(tc.kind, tc.color); got != tc.want {
			t.Errorf("Accepts(%s, %s) = %v, want %v", tc.kind, tc.color, got, tc.want)
		}
	}
	if got := strings.Join(Types(), ","); got != "expense,income,transfer" {
		t.Fatalf("Types = %s", got)
	}
	if !ValidType(Transfer) || ValidType("loan") {
		t.Fatal("ValidType is wrong")
	}
}

func TestTransactionHelpers(t *testing.T) {
	out := Transaction{Type: Transfer, Amount: -500, Tags: "#a #b"}
	if out.Value() != 500 || !out.Outgoing() {
		t.Fatalf("outgoing transfer: value %d, outgoing %v", out.Value(), out.Outgoing())
	}
	in := Transaction{Type: Income, Amount: 700}
	if in.Value() != 700 || in.Outgoing() {
		t.Fatalf("income: value %d, outgoing %v", in.Value(), in.Outgoing())
	}
	if got := out.TagList(); len(got) != 2 || got[0] != "#a" || got[1] != "#b" {
		t.Fatalf("TagList = %v", got)
	}
	if got := in.TagList(); len(got) != 0 {
		t.Fatalf("TagList of no tags = %v", got)
	}
}

func TestFilterMatch(t *testing.T) {
	market := View{
		Transaction: Transaction{Title: "Weekly Market", Type: Expense, Date: "2026-11-17", Tags: "#market #weekly"},
	}
	market.Account.Code, market.Category.Code = "INTER", "FOOD"
	for name, tc := range map[string]struct {
		filter Filter
		want   bool
	}{
		"empty":                 {Filter{}, true},
		"title substring":       {Filter{Title: "ly mar"}, true},
		"title other":           {Filter{Title: "rent"}, false},
		"category":              {Filter{Category: "food"}, true},
		"category part, exact":  {Filter{Category: "FOO"}, false},
		"category part, prefix": {Filter{Category: "FOO", Prefix: true}, true},
		"account":               {Filter{Account: "INTER"}, true},
		"account other":         {Filter{Account: "WISE"}, false},
		"account prefix":        {Filter{Account: "int", Prefix: true}, true},
		"type":                  {Filter{Type: Expense}, true},
		"type other":            {Filter{Type: Income}, false},
		"type prefix":           {Filter{Type: "exp", Prefix: true}, true},
		"tag":                   {Filter{Tags: []string{"#weekly"}}, true},
		"tag part, exact":       {Filter{Tags: []string{"#week"}}, false},
		"tag part, prefix":      {Filter{Tags: []string{"#week"}, Prefix: true}, true},
		"two tags":              {Filter{Tags: []string{"#weekly", "#market"}}, true},
		"one tag missing":       {Filter{Tags: []string{"#weekly", "#rent"}}, false},
		"year":                  {Filter{Period: "2026"}, true},
		"month":                 {Filter{Period: "2026-11"}, true},
		"other month":           {Filter{Period: "2026-10"}, false},
		"day":                   {Filter{Period: "2026-11-17"}, true},
		"all together":          {Filter{Title: "market", Category: "FOOD", Account: "INTER", Type: Expense, Tags: []string{"#market"}, Period: "2026-11"}, true},
		"all but one":           {Filter{Title: "market", Category: "FOOD", Account: "WISE"}, false},
		"not virtual":           {Filter{Virtual: "no"}, true},
		"virtual":               {Filter{Virtual: "yes"}, false},
		"virtual part":          {Filter{Virtual: "N", Prefix: true}, true},
	} {
		if got := tc.filter.Match(market); got != tc.want {
			t.Errorf("%s: Match = %v, want %v", name, got, tc.want)
		}
	}
	// A transaction of another person shows only when the filter does
	// not ask for mine, or names its account.
	hers := market
	hers.Account.Code, hers.Account.Owner = "ANA", "Ana"
	for name, tc := range map[string]struct {
		filter Filter
		want   bool
	}{
		"all":            {Filter{}, true},
		"mine":           {Filter{Mine: true}, false},
		"mine, account":  {Filter{Mine: true, Account: "ANA"}, true},
		"mine, other":    {Filter{Mine: true, Account: "INTER"}, false},
		"mine, category": {Filter{Mine: true, Category: "FOOD"}, false},
	} {
		if got := tc.filter.Match(hers); got != tc.want {
			t.Errorf("of another person, %s: Match = %v, want %v", name, got, tc.want)
		}
	}
	if !(Filter{Mine: true}).Match(market) {
		t.Error("the filter for mine drops my transaction")
	}
}

func TestParseSearch(t *testing.T) {
	got := ParseSearch("  weekly #mar @int cat:fo type:exp 2026-11 market ")
	if got.Title != "weekly market" || got.Category != "fo" || got.Account != "int" || got.Type != "exp" ||
		got.Period != "2026-11" || len(got.Tags) != 1 || got.Tags[0] != "#mar" || !got.Prefix {
		t.Fatalf("ParseSearch = %+v", got)
	}
	for input, want := range map[string]string{
		"11/2026":    "2026-11",
		"17/11/2026": "2026-11-17",
		"2026":       "2026",
		"2026-1":     "2026-1",
		"2026-11-1":  "2026-11-1",
	} {
		if got := ParseSearch(input); got.Period != want || got.Title != "" {
			t.Errorf("ParseSearch(%q) = %+v, want period %q", input, got, want)
		}
	}
	// A bare sign filters nothing, so typing a token does not empty the list.
	if got := ParseSearch("# @ cat: type:"); len(got.Tags) != 0 || got.Account != "" || got.Category != "" || got.Type != "" || got.Title != "" {
		t.Fatalf("empty tokens = %+v", got)
	}
	if got := ParseSearch("virtual:yes shop"); got.Virtual != "yes" || got.Title != "shop" {
		t.Fatalf("virtual token = %+v", got)
	}
	online := View{Transaction: Transaction{Title: "Shop", Virtual: true}}
	if !ParseSearch("virtual:y").Match(online) || ParseSearch("virtual:no").Match(online) || !ParseSearch("virtual:").Match(online) {
		t.Fatal("the virtual token does not select a virtual transaction")
	}
	if got := ParseSearch("offlimit:yes repair"); got.OffLimit != "yes" || got.Title != "repair" {
		t.Fatalf("offlimit token = %+v", got)
	}
	repair := View{Transaction: Transaction{Title: "Repair", OffLimit: true, Amount: -3000}}
	if !ParseSearch("offlimit:y").Match(repair) || ParseSearch("offlimit:no").Match(repair) || !ParseSearch("offlimit:").Match(repair) ||
		ParseSearch("offlimit:yes").Match(online) || !ParseSearch("offlimit:no").Match(online) {
		t.Fatal("the offlimit token does not select a transaction outside the limit")
	}
	if got := (Filter{OffLimit: "yes", Period: "2026"}).String(); got != "offlimit:yes 2026" {
		t.Fatalf("String with offlimit = %q", got)
	}
	if total, count := OffLimitTotal([]View{online, repair, repair}); total != 6000 || count != 2 {
		t.Fatalf("OffLimitTotal = %d, %d", total, count)
	}
	if got := (Filter{Virtual: "yes", Period: "2026"}).String(); got != "virtual:yes 2026" {
		t.Fatalf("String with virtual = %q", got)
	}
	if got := ParseSearch("50 abc"); got.Title != "50 abc" || got.Period != "" {
		t.Fatalf("plain words = %+v", got)
	}
	if got := (Filter{Category: "FOOD", Tags: []string{"#a"}, Period: "2026-11", Account: "INTER", Type: Expense, Title: "x y"}).String(); got != "x y cat:FOOD #a @INTER type:expense 2026-11" {
		t.Fatalf("String = %q", got)
	}
}
