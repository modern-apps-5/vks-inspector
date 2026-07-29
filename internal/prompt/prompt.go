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
		answer, err := p.readLine(promptLabel("select 1-"+itoa(len(choices)), def))
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

// Ask asks a free-text question. validate may be nil.
func (p *Prompter) Ask(question, def string, validate func(string) error) (string, error) {
	if !p.Interactive {
		if def != "" {
			return def, nil
		}
		return "", fmt.Errorf("%w: %s", ErrNonInteractive, question)
	}

	for {
		answer, err := p.readLine(promptLabel(question, def))
		if err != nil {
			return "", err
		}
		if answer == "" {
			if def == "" {
				fmt.Fprintf(p.out, "     required\n")
				continue
			}
			answer = def
		}
		if validate != nil {
			if err := validate(answer); err != nil {
				fmt.Fprintf(p.out, "     %v\n", err)
				continue
			}
		}
		return answer, nil
	}
}

// AskOptional is Ask that accepts an empty answer.
func (p *Prompter) AskOptional(question string, validate func(string) error) (string, error) {
	if !p.Interactive {
		return "", nil
	}
	for {
		answer, err := p.readLine(promptLabel(question+" (optional)", ""))
		if err != nil {
			return "", err
		}
		if answer == "" {
			return "", nil
		}
		if validate != nil {
			if err := validate(answer); err != nil {
				fmt.Fprintf(p.out, "     %v\n", err)
				continue
			}
		}
		return answer, nil
	}
}

// AskList asks for a comma-separated list.
func (p *Prompter) AskList(question string, def []string, validate func(string) error) ([]string, error) {
	defStr := strings.Join(def, ", ")
	answer, err := p.Ask(question, defStr, func(s string) error {
		if validate == nil {
			return nil
		}
		for _, item := range splitList(s) {
			if err := validate(item); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return splitList(answer), nil
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

func promptLabel(question, def string) string {
	if def != "" {
		return fmt.Sprintf("  %s [%s]: ", question, def)
	}
	return fmt.Sprintf("  %s: ", question)
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
