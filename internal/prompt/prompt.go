// Package prompt is the interactive question flow.
//
// The design rule that matters: **prompting produces a config, it is not an
// alternative to one.** The answers are assembled into exactly the same
// config.Config a YAML file would produce, and can be written out with
// --save-config. That is what keeps verify, snapshot and drift — which run in
// pipelines and cannot prompt — working off the same declared intent as an
// interactive first run.
//
// Consequences that follow from that rule and must not be quietly broken:
//
//   - a prompt never asks for something already supplied by flag or config;
//   - a prompt never asks for something that can be discovered from vCenter;
//   - --non-interactive turns every unanswered question into a clear error
//     naming the config field, never a silent default. A default that is
//     silently wrong produces a confident wrong verdict, which is the failure
//     mode this whole tool exists to prevent;
//   - nothing here ever asks for a password. Credentials come from the
//     environment or the credentials file. See docs/ADR/0005.
package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrNonInteractive is returned when input is needed but prompting is disabled.
var ErrNonInteractive = errors.New("input required but prompting is disabled")

// Prompter asks questions on a terminal.
type Prompter struct {
	in  *bufio.Reader
	out io.Writer
	// Interactive is false in CI or under --non-interactive. Every Ask then
	// fails rather than defaulting.
	Interactive bool
	// pending holds a declared-but-unprinted section heading.
	pending string
}

// New returns a Prompter reading from in and writing to out.
func New(in io.Reader, out io.Writer, interactive bool) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out, Interactive: interactive}
}

// Choice is one option in a select prompt.
type Choice struct {
	Value string
	Label string
	// Note is a caveat shown alongside the option — used for the topology
	// combinations the requirements matrix flags as unverified. An operator
	// choosing HAProxy should see that it may not be supported at the moment
	// they choose it, not three screens later in a report.
	Note string
}

// Select asks the user to pick one of a fixed set of options.
//
// If there is exactly one option it is chosen without asking. Asking a question
// with one possible answer wastes the operator's attention, which is the scarce
// resource during a change window.
func (p *Prompter) Select(question string, choices []Choice, def string) (string, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("no options for %q", question)
	}
	if len(choices) == 1 {
		return choices[0].Value, nil
	}
	if !p.Interactive {
		if def != "" {
			return def, nil
		}
		return "", fmt.Errorf("%w: %s", ErrNonInteractive, question)
	}

	p.flushSection()
	fmt.Fprintf(p.out, "\n  %s\n", question)
	for i, c := range choices {
		marker := " "
		if c.Value == def {
			marker = "*"
		}
		label := c.Label
		if label == "" {
			label = c.Value
		}
		fmt.Fprintf(p.out, "   %s %d) %-14s %s\n", marker, i+1, c.Value, label)
		if c.Note != "" {
			fmt.Fprintf(p.out, "        %s %s\n", flagMark, c.Note)
		}
	}

	for {
		answer, err := p.readLine(promptLabel("select 1-"+itoa(len(choices)), "", def))
		if err != nil {
			return "", err
		}
		if answer == "" && def != "" {
			return def, nil
		}
		// Accept either the number or the value itself — an operator who knows
		// the tool should not have to count.
		for i, c := range choices {
			if answer == itoa(i+1) || strings.EqualFold(answer, c.Value) {
				return c.Value, nil
			}
		}
		fmt.Fprintf(p.out, "     not one of the options\n")
	}
}

// Validator checks an answer and returns it in canonical form.
//
// It returns the normalised value rather than just an error so a prompt can
// accept what an operator naturally types and store what the config schema
// needs — "192.168.200.5" becoming "192.168.200.5/32", for instance. Rejecting
// input the tool could have understood is friction with nothing to show for it.
type Validator func(string) (string, error)

// Ask asks a free-text question. validate may be nil.
//
// example is shown as "e.g. …" and is not optional in spirit: a free-text
// prompt with no example makes the operator guess at the format, and they will
// guess wrong. Pass "" only when the shape is genuinely self-evident.
func (p *Prompter) Ask(question, example, def string, validate Validator) (string, error) {
	if !p.Interactive {
		if def != "" {
			return def, nil
		}
		return "", fmt.Errorf("%w: %s", ErrNonInteractive, question)
	}

	for {
		answer, err := p.readLine(promptLabel(question, example, def))
		if err != nil {
			return "", err
		}
		if answer == "" {
			if def == "" {
				fmt.Fprintf(p.out, "     required — %s\n", requiredHint(example))
				continue
			}
			answer = def
		}
		if validate != nil {
			normalised, err := validate(answer)
			if err != nil {
				fmt.Fprintf(p.out, "     %v\n", err)
				continue
			}
			answer = normalised
		}
		return answer, nil
	}
}

// AskOptional is Ask that accepts an empty answer.
func (p *Prompter) AskOptional(question, example string, validate Validator) (string, error) {
	if !p.Interactive {
		return "", nil
	}
	for {
		answer, err := p.readLine(promptLabel(question+" (optional)", example, ""))
		if err != nil {
			return "", err
		}
		if answer == "" {
			return "", nil
		}
		if validate != nil {
			normalised, err := validate(answer)
			if err != nil {
				fmt.Fprintf(p.out, "     %v\n", err)
				continue
			}
			answer = normalised
		}
		return answer, nil
	}
}

// AskList asks for a comma-separated list. At least one entry is required.
func (p *Prompter) AskList(question, example string, def []string, validate Validator) ([]string, error) {
	return p.askList(question, example, def, validate, false)
}

// AskListOptional asks for a comma-separated list and accepts an empty answer.
//
// Separate from AskList rather than a bool argument because the distinction is
// load-bearing: a prompt whose text says "leave empty to skip" and then rejects
// an empty answer is the tool lying about its own behaviour, and that is worse
// than an awkward API.
func (p *Prompter) AskListOptional(question, example string, validate Validator) ([]string, error) {
	return p.askList(question, example, nil, validate, true)
}

func (p *Prompter) askList(question, example string, def []string, validate Validator, allowEmpty bool) ([]string, error) {
	defStr := strings.Join(def, ", ")

	if !p.Interactive {
		if defStr != "" {
			return splitList(defStr), nil
		}
		if allowEmpty {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %s", ErrNonInteractive, question)
	}

	for {
		answer, err := p.readLine(promptLabel(question, example, defStr))
		if err != nil {
			return nil, err
		}
		if answer == "" {
			if defStr != "" {
				answer = defStr
			} else if allowEmpty {
				return nil, nil
			} else {
				fmt.Fprintf(p.out, "     required — %s\n", requiredHint(example))
				continue
			}
		}

		items := splitList(answer)
		out := make([]string, 0, len(items))
		bad := false
		for _, item := range items {
			if validate == nil {
				out = append(out, item)
				continue
			}
			normalised, err := validate(item)
			if err != nil {
				fmt.Fprintf(p.out, "     %v\n", err)
				bad = true
				break
			}
			out = append(out, normalised)
		}
		if bad {
			continue
		}
		return out, nil
	}
}

// Confirm asks a yes/no question.
func (p *Prompter) Confirm(question string, def bool) (bool, error) {
	if !p.Interactive {
		return def, nil
	}
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		answer, err := p.readLine(fmt.Sprintf("  %s [%s]: ", question, hint))
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}

// Section declares a heading. It is printed lazily, only if a question in it is
// actually asked.
//
// Eager printing produced empty headings for whole sections whose values were
// already supplied by config or flags — a run that asked nothing still emitted
// "Addressing" and "NSX" with nothing under them. Noise in an interactive tool
// is not cosmetic: it trains the operator to stop reading.
func (p *Prompter) Section(title string) { p.pending = title }

// flushSection prints any deferred heading. Called immediately before anything
// is written to the terminal.
func (p *Prompter) flushSection() {
	if p.pending == "" || !p.Interactive {
		return
	}
	title := p.pending
	p.pending = ""
	fmt.Fprintf(p.out, "\n%s\n%s\n", title, strings.Repeat("─", runeLen(title)))
}

func runeLen(s string) int { return len([]rune(s)) }

// Info prints a line of context without asking anything. Used to report what
// was discovered from vCenter, so the operator can see which questions were
// answered for them and correct one that is wrong.
func (p *Prompter) Info(format string, args ...any) {
	if !p.Interactive {
		return
	}
	p.flushSection()
	fmt.Fprintf(p.out, "  "+format+"\n", args...)
}

func (p *Prompter) readLine(label string) (string, error) {
	p.flushSection()
	fmt.Fprint(p.out, label)
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%w: input ended", ErrNonInteractive)
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

const flagMark = "⚑"

func promptLabel(question, example, def string) string {
	if example != "" {
		question += "  (e.g. " + example + ")"
	}
	if def != "" {
		return fmt.Sprintf("  %s [%s]: ", question, def)
	}
	return fmt.Sprintf("  %s: ", question)
}

// requiredHint turns a bare "required" into something actionable. Being told
// "required" three times in a row without being told what shape the answer
// takes is how an operator ends up guessing.
func requiredHint(example string) string {
	if example == "" {
		return "this question has no default"
	}
	return "expected something like " + example
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
