package prompt_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/modern-apps-5/vks-inspector/internal/prompt"
)

// scripted drives a Prompter with canned answers and captures what it printed.
func scripted(answers ...string) (*prompt.Prompter, *strings.Builder) {
	in := strings.NewReader(strings.Join(answers, "\n") + "\n")
	out := &strings.Builder{}
	return prompt.New(in, out, true), out
}

// The bug this file exists for: a prompt whose text says "leave empty to skip"
// must actually accept an empty answer. Rejecting it is the tool lying about
// its own behaviour, and it strands the operator with no way forward.
func TestAskListOptionalAcceptsEmpty(t *testing.T) {
	t.Parallel()

	p, out := scripted("")
	got, err := p.AskListOptional("Existing networks", "10.0.0.0/8", nil)
	if err != nil {
		t.Fatalf("empty answer should be accepted, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
	if strings.Contains(out.String(), "required") {
		t.Error(`an optional prompt must never print "required"`)
	}
}

// The counterpart: a genuinely required list still refuses empty, and says what
// shape it wants rather than only "required".
func TestAskListRequiredRefusesEmptyAndShowsTheFormat(t *testing.T) {
	t.Parallel()

	p, out := scripted("", "10.0.0.53")
	got, err := p.AskList("DNS servers", "10.10.0.53", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "10.0.0.53" {
		t.Errorf("got %v", got)
	}
	if !strings.Contains(out.String(), "required") {
		t.Error("a required prompt should say so")
	}
	// "required" three times with no hint is how an operator ends up guessing.
	if !strings.Contains(out.String(), "expected something like 10.10.0.53") {
		t.Errorf("the required message should show the expected shape, got:\n%s", out)
	}
}

// Every free-text prompt should show an example. Being told "required" without
// being told the format is the failure this suite guards against.
func TestExampleIsShownInTheLabel(t *testing.T) {
	t.Parallel()

	p, out := scripted("lab-01")
	if _, err := p.Ask("Name", "lab-nsx-01", "", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(e.g. lab-nsx-01)") {
		t.Errorf("example not shown in prompt label, got:\n%s", out)
	}
}

func TestValidatorNormalisesTheAnswer(t *testing.T) {
	t.Parallel()

	upper := func(s string) (string, error) { return strings.ToUpper(s), nil }

	p, _ := scripted("abc")
	got, err := p.Ask("Thing", "", "", upper)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ABC" {
		t.Errorf("got %q, want the normalised value ABC", got)
	}
}

func TestValidatorRejectionRepromptsRatherThanFailing(t *testing.T) {
	t.Parallel()

	reject := func(s string) (string, error) {
		if s == "good" {
			return s, nil
		}
		return "", errors.New("nope")
	}

	p, out := scripted("bad", "worse", "good")
	got, err := p.Ask("Thing", "", "", reject)
	if err != nil {
		t.Fatal(err)
	}
	if got != "good" {
		t.Errorf("got %q", got)
	}
	if strings.Count(out.String(), "nope") != 2 {
		t.Errorf("expected two rejections to be reported, got:\n%s", out)
	}
}

// A rejected entry must re-prompt for the whole list, not silently keep the
// entries before it.
func TestListRejectionRepromptsForTheWholeList(t *testing.T) {
	t.Parallel()

	noBare := func(s string) (string, error) {
		if strings.Contains(s, "/") {
			return s, nil
		}
		return "", errors.New("needs a prefix")
	}

	p, _ := scripted("10.0.0.0/8, oops", "10.0.0.0/8, 172.16.0.0/12")
	got, err := p.AskList("Nets", "10.0.0.0/8", nil, noBare)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "10.0.0.0/8" || got[1] != "172.16.0.0/12" {
		t.Errorf("got %v", got)
	}
}

// Non-interactive must error naming the question rather than defaulting.
// A silently-wrong default produces a confident wrong verdict, which is the
// failure mode this whole tool exists to prevent.
func TestNonInteractiveErrorsInsteadOfDefaulting(t *testing.T) {
	t.Parallel()

	p := prompt.New(strings.NewReader(""), io.Discard, false)

	if _, err := p.Ask("Management CIDR", "10.0.0.0/24", "", nil); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Errorf("Ask: got %v, want ErrNonInteractive", err)
	}
	if _, err := p.AskList("DNS servers", "10.0.0.53", nil, nil); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Errorf("AskList: got %v, want ErrNonInteractive", err)
	}
	// A declared default is a legitimate answer, not a guess.
	if got, err := p.Ask("Skew", "", "30", nil); err != nil || got != "30" {
		t.Errorf("Ask with default: got %q, %v", got, err)
	}
	// An optional list is legitimately empty without input.
	if got, err := p.AskListOptional("External nets", "10.0.0.0/8", nil); err != nil || len(got) != 0 {
		t.Errorf("AskListOptional: got %v, %v", got, err)
	}
}

// Running out of input must be a clear error, not a hang or a silent default.
func TestExhaustedInputIsAClearError(t *testing.T) {
	t.Parallel()

	p := prompt.New(strings.NewReader(""), io.Discard, true)
	if _, err := p.Ask("Thing", "x", "", nil); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Errorf("got %v, want ErrNonInteractive", err)
	}
}

// A single-option question is answered without asking. Spending the operator's
// attention on a question with one possible answer is a cost with no benefit.
func TestSelectSkipsWhenThereIsOnlyOneOption(t *testing.T) {
	t.Parallel()

	p, out := scripted()
	got, err := p.Select("Load balancer", []prompt.Choice{{Value: "alb"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "alb" {
		t.Errorf("got %q", got)
	}
	if out.String() != "" {
		t.Errorf("nothing should have been printed, got:\n%s", out)
	}
}

func TestSelectAcceptsNumberOrValue(t *testing.T) {
	t.Parallel()

	choices := []prompt.Choice{{Value: "vds"}, {Value: "nsx"}, {Value: "nsx-vpc"}}

	for _, answer := range []string{"2", "nsx", "NSX"} {
		p, _ := scripted(answer)
		got, err := p.Select("Networking", choices, "")
		if err != nil {
			t.Fatalf("%q: %v", answer, err)
		}
		if got != "nsx" {
			t.Errorf("%q selected %q, want nsx", answer, got)
		}
	}
}

// Caveats attached to an option must be visible at the moment of choosing, not
// three screens later in the report.
func TestSelectShowsOptionCaveats(t *testing.T) {
	t.Parallel()

	p, out := scripted("1")
	_, err := p.Select("Load balancer", []prompt.Choice{
		{Value: "haproxy", Note: "believed removed in VCF 9"},
		{Value: "alb"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "believed removed in VCF 9") {
		t.Errorf("caveat not surfaced, got:\n%s", out)
	}
}

// Section headings print only when a question under them is actually asked.
// Eager printing emitted empty headings for sections already answered by
// config, and noise trains the operator to stop reading.
func TestSectionHeadingIsLazy(t *testing.T) {
	t.Parallel()

	p, out := scripted()
	p.Section("Addressing")
	if out.String() != "" {
		t.Errorf("heading printed with no question asked:\n%s", out)
	}

	p2, out2 := scripted("x")
	p2.Section("Addressing")
	if _, err := p2.Ask("Thing", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2.String(), "Addressing") {
		t.Errorf("heading not printed before its question:\n%s", out2)
	}
}
