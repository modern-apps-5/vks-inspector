package renderers

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// JUnit renders JUnit XML for CI collectors.
//
// The mapping is opinionated and worth stating: a failed blocker is a
// <failure>, a failed warning is also a <failure> (CI tools have no concept of
// severity, and silently passing a warning would make the CI view disagree with
// the exit code), an indeterminate result is a <failure> with a distinct type,
// a tool error is an <error>, and a skip is <skipped>. Anything else and the
// CI dashboard tells a different story from the terminal.
type JUnit struct {
	Opts Options
}

// Name implements Renderer.
func (j *JUnit) Name() string { return "junit" }

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Time     float64      `xml:"time,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Errors   int         `xml:"errors,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Error     *junitFailure `xml:"error,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// Render implements Renderer.
//
// TODO(phase-2): group cases into one <testsuite> per category rather than a
// single suite, once there are enough checks for that grouping to help. The
// element structure below is already suite-per-category shaped so that change
// does not break consumers.
func (j *JUnit) Render(w io.Writer, rep *results.Report) error {
	suite := junitSuite{Name: "vksinspect." + rep.Run.Mode}

	for _, r := range bySeverityThenID(rep.Results) {
		c := junitCase{
			Name:      r.Title,
			Classname: "vksinspect." + string(r.Category) + "." + r.CheckID,
			Time:      float64(r.DurationMS) / 1000.0,
			SystemOut: fmt.Sprintf("expected: %s\nobserved: %s\n", r.Expected.Summary, r.Observed.Summary),
		}
		switch r.Status {
		case results.StatusFail:
			c.Failure = &junitFailure{
				Message: r.Observed.Summary,
				Type:    string(r.Severity),
				Body:    fmt.Sprintf("expected: %s\nobserved: %s\nremediation: %s", r.Expected.Summary, r.Observed.Summary, r.Remediation),
			}
			suite.Failures++
		case results.StatusUnknown:
			c.Failure = &junitFailure{
				Message: r.Observed.Summary,
				Type:    "indeterminate",
				Body:    "the check ran but could not determine an answer; this is not proof of health",
			}
			suite.Failures++
		case results.StatusError:
			c.Error = &junitFailure{Message: r.Err, Type: "tool-error", Body: r.Err}
			suite.Errors++
		case results.StatusSkip:
			c.Skipped = &junitSkipped{Message: r.Observed.Summary}
			suite.Skipped++
		}
		suite.Tests++
		suite.Cases = append(suite.Cases, c)
	}

	doc := junitSuites{
		Name:     "vksinspect",
		Tests:    suite.Tests,
		Failures: suite.Failures,
		Errors:   suite.Errors,
		Skipped:  suite.Skipped,
		Time:     float64(rep.Run.DurationMS) / 1000.0,
		Suites:   []junitSuite{suite},
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}
