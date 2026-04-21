package app

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderSummaryBannerSuccess(t *testing.T) {
	t.Parallel()

	got := stripANSI(renderSummaryBanner(summaryStateSuccess))
	if !strings.Contains(got, "# SUMMARY") {
		t.Fatalf("expected summary banner label, got:\n%s", got)
	}
	if strings.Count(got, "# ================================================") != 2 {
		t.Fatalf("expected two separator lines, got:\n%s", got)
	}
}

func TestRenderSummaryBannerWarningUsesYellowSummary(t *testing.T) {
	t.Parallel()

	got := renderSummaryBanner(summaryStateWarning)
	want := formatTitle("# ", true) + colorYellow + "SUMMARY" + colorReset
	if !strings.Contains(got, want) {
		t.Fatalf("expected yellow SUMMARY, got:\n%q", got)
	}
}

func TestRenderSummaryBannerErrorUsesRedSummary(t *testing.T) {
	t.Parallel()

	got := renderSummaryBanner(summaryStateError)
	want := formatTitle("# ", true) + colorRed + "SUMMARY" + colorReset
	if !strings.Contains(got, want) {
		t.Fatalf("expected red SUMMARY, got:\n%q", got)
	}
}

func TestRenderSummaryBannerBlockStartsWithBlankLine(t *testing.T) {
	t.Parallel()

	got := stripANSI(renderSummaryBannerBlock(summaryStateSuccess))
	if !strings.HasPrefix(got, "\n# ================================================") {
		t.Fatalf("expected summary block to start with a blank line, got:\n%q", got)
	}
}

func TestRenderSummaryFooter(t *testing.T) {
	t.Parallel()

	got := stripANSI(renderSummaryFooter())
	if got != "# ================================================" {
		t.Fatalf("renderSummaryFooter() = %q", got)
	}
}

func TestRenderSummaryLinePrefixesGreenHash(t *testing.T) {
	t.Parallel()

	got := renderSummaryLine("email domains missing on destination side: voplmopple.com", colorYellow)
	wantPrefix := formatTitle("# ", true)
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("expected summary line to start with green hash, got:\n%q", got)
	}
	if !strings.Contains(got, colorYellow+"voplmopple.com"+colorReset) {
		t.Fatalf("expected colored suffix in summary line, got:\n%q", got)
	}
}

func TestSummaryStateFromOutcomeReturnsErrorWhenWorkflowFailed(t *testing.T) {
	t.Parallel()

	if got := summaryStateFromOutcome(nil, errors.New("boom")); got != summaryStateError {
		t.Fatalf("summaryStateFromOutcome(nil, err) = %v, want %v", got, summaryStateError)
	}
}

func TestSummaryStateFromOutcomeReturnsErrorWhenFailuresExist(t *testing.T) {
	t.Parallel()

	if got := summaryStateFromOutcome([]remoteFailure{{Action: "x", Reason: "y"}}, nil); got != summaryStateError {
		t.Fatalf("summaryStateFromOutcome(failures, nil) = %v, want %v", got, summaryStateError)
	}
}

func TestSummaryStateFromOutcomeReturnsWarningWithoutFailuresOrError(t *testing.T) {
	t.Parallel()

	if got := summaryStateFromOutcome(nil, nil); got != summaryStateWarning {
		t.Fatalf("summaryStateFromOutcome(nil, nil) = %v, want %v", got, summaryStateWarning)
	}
}

func TestCompareSourceToRemoteDoesNotReportStandaloneDBUsers(t *testing.T) {
	t.Parallel()

	source := SourceData{
		DBUsers: []DBUser{
			{ID: "1", Name: "dfdfd", Server: "mariadb-10.0"},
		},
	}

	diffs := compareSourceToRemote(source, SourceData{}, "all")
	for _, diff := range diffs {
		if diff.title == "db users" {
			t.Fatalf("did not expect standalone db users diff, got %#v", diffs)
		}
	}
}

func TestCompareSourceToRemoteReportsMissingDBServersWithoutDBUserDuplication(t *testing.T) {
	t.Parallel()

	source := SourceData{
		DBServers: []DBServer{
			{ID: "1", Name: "mariadb-10.6"},
		},
		DBUsers: []DBUser{
			{ID: "2", Name: "dfdfd", Server: "mariadb-10.0"},
		},
	}

	diffs := compareSourceToRemote(source, SourceData{}, "all")
	if len(diffs) != 1 {
		t.Fatalf("expected only one diff, got %#v", diffs)
	}
	if diffs[0].title != "database servers" {
		t.Fatalf("expected only database servers diff, got %#v", diffs)
	}
	if len(diffs[0].values) != 1 || diffs[0].values[0] != "mariadb-10.6" {
		t.Fatalf("expected mariadb-10.6 diff, got %#v", diffs)
	}
}
