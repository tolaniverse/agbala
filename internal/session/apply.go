package session

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// The tags the design gives each event class. They are presentation, which is
// why they live here rather than on the wire.
const (
	tagHost     = "HOST"
	tagOfin     = "ÒFIN"
	tagAgent    = "AGENT"
	tagYou      = "YOU"
	tagTool     = "TOOL"
	tagProposed = "PROPOSED"
	tagDenied   = "DENIED"
	tagApproval = "APPROVAL"
	tagForks    = "FORKS"
)

// ---------------------------------------------------------------- lifecycle

func (s *State) applySessionStarted(p event.SessionStarted) {
	s.Session.Command = "agbala attach " + p.Sandbox
	s.Cwd, s.Branch = p.Cwd, p.Branch
	s.Rail.Model = p.Model
	s.ctxLimit = p.CtxLimit
	if p.CtxLimit > 0 {
		s.Rail.CtxLabel = contextLabel(0, p.CtxLimit)
	}
}

func (s *State) applyTurnState(p event.TurnState) {
	s.Session.State = p.State
	s.Session.Tone = stateTone(p.State)
	// A forked session is doing several things at once, and the design says so
	// in the chip rather than making you read the rail to find out.
	if s.forked && p.State == "EXECUTING" {
		s.Session.Tone = theme.ToneFork
	}
	if p.Turn > 0 {
		s.Session.Turn = fmt.Sprintf("%d", p.Turn)
	}
	s.Loading = p.Detail
	if p.Dirty != "" {
		s.Dirty = p.Dirty
	}
	if m, ok := parseMode(p.Mode); ok {
		s.Mode = m
	}
}

// stateTone colours the status chip. EXECUTING is green because the loop is
// doing what you asked; AWAITING_APPROVAL is yellow because it is waiting on
// you, and that distinction is the one worth seeing from across a room.
func stateTone(state string) theme.Tone {
	switch state {
	case "EXECUTING", "DONE":
		return theme.ToneOK
	case "AWAITING_APPROVAL":
		return theme.ToneHuman
	case "FAILED":
		return theme.ToneDeny
	default:
		return theme.ToneAgent
	}
}

func parseMode(s string) (theme.Mode, bool) {
	for _, m := range theme.Modes() {
		if m.String() == s {
			return m, true
		}
	}
	return theme.ModePlan, false
}

func (s *State) applyHostLog(p event.HostLog) {
	i := s.block(p.Block, tagHost, theme.ToneSys, p.Meta)

	rows := make([][]string, 0, len(p.Steps))
	for _, step := range p.Steps {
		label := step.Label
		if p.Marks {
			mark := "✔"
			if !step.OK {
				mark = "✕"
			}
			label = mark + " " + label
		}
		rows = append(rows, []string{label, step.Value, step.Note})
	}
	var formatted []string
	if p.Marks {
		formatted = columnsAt(rows, hostStops, wideGap)
	} else {
		// A snapshot note carries no marks and sits closer in, but aligns the
		// same way: first column to a stop, the rest spaced.
		formatted = columnsAt(rows, noteStops, minGap)
	}
	lines := make([]ui.Line, 0, len(rows))
	for _, row := range formatted {
		lines = append(lines, ui.Line{Text: row, Color: theme.TextDim})
	}
	s.Blocks[i].Lines = lines
	s.touch(p.Block)
}

// ------------------------------------------------------------- conversation

func (s *State) applyUserMessage(p event.UserMessage) {
	i := s.block(p.Block, tagYou, theme.ToneUser, p.At)
	s.Blocks[i].Lines = []ui.Line{{Text: p.Text, Color: theme.TextBright, Wrap: true}}
	s.touch(p.Block)
}

func (s *State) applyAgentMessage(p event.AgentMessage) {
	i := s.block(p.Block, tagAgent, theme.ToneAgent, p.Meta)
	if p.Meta != "" {
		s.Blocks[i].Meta = p.Meta
	}
	s.Blocks[i].Lines = paragraphs(p.Text)
	s.touch(p.Block)
}

// applyAgentDelta appends a chunk to a message still being generated.
//
// The text is re-split on every delta rather than appended to the last line:
// a chunk can arrive mid-word or carry a newline, and rebuilding is the only
// way the result matches what a completed message would have rendered as.
func (s *State) applyAgentDelta(p event.AgentDelta) {
	i := s.block(p.Block, tagAgent, theme.ToneAgent, "")
	var sb strings.Builder
	for n, line := range s.Blocks[i].Lines {
		if n > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line.Text)
	}
	sb.WriteString(p.Text)
	s.Blocks[i].Lines = paragraphs(sb.String())
	s.touch(p.Block)
}

// paragraphs splits prose into lines the renderer will wrap.
func paragraphs(text string) []ui.Line {
	parts := strings.Split(text, "\n")
	out := make([]ui.Line, 0, len(parts))
	for _, part := range parts {
		out = append(out, ui.Line{Text: part, Color: theme.TextNormal, Wrap: true})
	}
	return out
}

func (s *State) applyAgentPlan(p event.AgentPlan) {
	i := s.block(p.Block, tagAgent, theme.ToneAgent, planMeta(len(p.Steps)))
	s.Blocks[i].Meta = planMeta(len(p.Steps))

	lines := make([]ui.Line, 0, len(p.Steps))
	for n, step := range p.Steps {
		lines = append(lines, ui.Line{
			Text:  fmt.Sprintf("%d  %s", n+1, step),
			Color: theme.TextBody, Wrap: true,
		})
	}
	s.Blocks[i].Lines = lines
	s.touch(p.Block)
}

func planMeta(n int) string {
	if n == 1 {
		return "plan · 1 step"
	}
	return fmt.Sprintf("plan · %d steps", n)
}

// ------------------------------------------------------- tools and the gate

func (s *State) applyToolProposed(p event.ToolProposed) {
	i := s.block(p.Block, tagProposed, theme.ToneTool, "tool_call · pending gate")

	var lines []ui.Line
	rows := make([][]string, 0, len(p.Calls))
	for _, c := range p.Calls {
		rows = append(rows, []string{c.Tool, c.Args})
	}
	formatted := columnsAt(rows, toolStops, wideGap)
	for n, c := range p.Calls {
		lines = append(lines, ui.Line{Text: formatted[n], Color: theme.TextBody})
		if c.Detail != "" {
			lines = append(lines, ui.Line{Text: c.Detail, Color: theme.TextDim, Indent: 2})
		}
	}
	s.Blocks[i].Lines = lines
	s.proposed[p.Block] = toolNames(p.Calls)
	s.touch(p.Block)
}

// toolNames lists the distinct tools a proposal would run, in order, which is
// how the design labels the block: "read", "write, edit".
func toolNames(calls []event.ToolCall) string {
	var names []string
	seen := map[string]bool{}
	for _, c := range calls {
		if seen[c.Tool] {
			continue
		}
		seen[c.Tool] = true
		names = append(names, c.Tool)
	}
	return strings.Join(names, ", ")
}

func (s *State) applyOfinScope(p event.OfinScope) {
	s.Rail.RuleCount = ruleCount(p.Total)
	s.Rail.Rules = nil
	for _, r := range p.Rules {
		s.Rail.Rules = append(s.Rail.Rules, ui.Rule{ID: r.ID, Text: r.Summary})
	}
	if p.Block == "" {
		return
	}

	i := s.block(p.Block, tagOfin, theme.ToneAgent, "scope: "+p.Scope)
	s.Blocks[i].Meta = "scope: " + p.Scope

	lines := []ui.Line{{
		Text:  fmt.Sprintf("%d rules resolved and injected into the system prompt", p.Total),
		Color: theme.TextBody, Wrap: true,
	}}
	rows := make([][]string, 0, len(p.Rules))
	for _, r := range p.Rules {
		rows = append(rows, []string{"ofin-" + r.ID, r.Tool, r.Match, r.Verdict})
	}
	for _, row := range columnsAt(rows, ofinStops, wideGap) {
		lines = append(lines, ui.Line{Text: row, Color: theme.TextDim, Indent: 2})
	}
	if rest := p.Total - len(p.Rules); rest > 0 {
		lines = append(lines, ui.Line{
			Text:  fmt.Sprintf("… %d more   ·   agbala ofin rules", rest),
			Color: theme.TextLabel, Indent: 2,
		})
	}
	s.Blocks[i].Lines = lines
	s.touch(p.Block)
}

// applyOfinVerdict renders the gate's decision.
//
// A denial is its own block and the turn continues — it returns to the agent as
// an observation, never as a modal or a turn-ender. Only require_human suspends
// anything, and that is what claims the input line.
func (s *State) applyOfinVerdict(p event.OfinVerdict) error {
	if !p.Verdict.Valid() {
		return fmt.Errorf("unknown verdict %q for proposal %s", p.Verdict, p.Proposal)
	}

	// An allowed call becomes the tool block it was proposing.
	if p.Verdict == event.VerdictAllow {
		if i, ok := s.order[p.Proposal]; ok {
			s.Blocks[i].Tag = tagTool
			s.Blocks[i].Meta = toolMeta(s.proposed[p.Proposal], "running")
			// The block stops being a proposal and becomes the run. What it
			// was going to do is superseded by what it is doing, and the gate's
			// decision is in the log either way.
			s.Blocks[i].Lines = nil
			s.touch(p.Proposal)
		}
		s.hotRule(p.RuleID, false)
		return nil
	}

	tag, tone := tagDenied, theme.ToneDeny
	if p.Verdict == event.VerdictRequireHuman {
		tag, tone = tagApproval, theme.ToneHuman
	}
	i := s.block(p.Block, tag, tone, verdictMeta(p))
	s.Blocks[i].Meta = verdictMeta(p)

	accent := theme.Deny
	if p.Verdict == event.VerdictRequireHuman {
		accent = theme.Human
	}
	var lines []ui.Line
	if p.Headline != "" {
		lines = append(lines, ui.Line{Text: p.Headline, Color: accent, Bold: true, Boxed: true})
	}
	if p.Rationale != "" {
		lines = append(lines, ui.Line{Text: p.Rationale, Color: theme.TextMuted, Boxed: true, Wrap: true})
	}
	for _, term := range p.Terms {
		lines = append(lines, ui.Line{Text: term, Color: theme.TextSubtle, Boxed: true})
	}

	switch p.Verdict {
	case event.VerdictDeny:
		// The note the design puts outside the box: the loop is still running.
		lines = append(lines, ui.Line{
			Text:  "returned to the agent as an observation · turn continues",
			Color: theme.TextLabel, Italic: true,
		})
		s.Approval = nil
	case event.VerdictRequireHuman:
		if len(p.Options) > 0 {
			var keys []string
			for _, o := range p.Options {
				keys = append(keys, fmt.Sprintf("[%s] %s", o.Key, o.Label))
			}
			lines = append(lines, ui.Line{
				Text: strings.Join(keys, "   "), Color: theme.Human, Boxed: true,
			})
		}
		s.Approval = &Approval{Proposal: p.Proposal, RuleID: p.RuleID, Options: p.Options}
	}

	s.Blocks[i].Lines = lines
	s.touch(p.Block)
	s.hotRule(p.RuleID, true)
	return nil
}

func verdictMeta(p event.OfinVerdict) string {
	parts := []string{}
	if p.RuleID != "" {
		parts = append(parts, "ofin-"+p.RuleID)
	}
	if p.RuleVer != "" {
		parts = append(parts, p.RuleVer)
	}
	switch {
	case p.Verdict == event.VerdictRequireHuman:
		parts = append(parts, "require_human")
	case p.GateMicros > 0:
		parts = append(parts, fmt.Sprintf("gate %dms", p.GateMicros/1000))
	}
	return strings.Join(parts, " · ")
}

// hotRule marks the rule that just fired, so the rail pulls it forward.
func (s *State) hotRule(id string, hot bool) {
	for i := range s.Rail.Rules {
		s.Rail.Rules[i].Hot = hot && s.Rail.Rules[i].ID == id
	}
}

func (s *State) applyToolOutput(p event.ToolOutput) {
	i := s.block(p.Block, tagTool, theme.ToneTool, "")
	for _, l := range p.Lines {
		s.Blocks[i].Lines = append(s.Blocks[i].Lines, ui.Line{
			Text: l.Text, Color: outputColor(l.Status),
		})
		switch l.Status {
		case "err":
			s.outcome[p.Block] = theme.ToneDeny
		case "ok":
			if _, settled := s.outcome[p.Block]; !settled {
				s.outcome[p.Block] = theme.ToneOK
			}
		}
	}
	s.Blocks[i].Tone = s.toneFor(p.Block, theme.ToneTool)
	s.touch(p.Block)
}

// toolMeta labels a tool block the way the design does: which tools ran, that
// the gate allowed them, and how it went.
func toolMeta(tools, status string) string {
	if tools == "" {
		return "allow · " + status
	}
	return tools + " · allow · " + status
}

// toneFor returns what a tool block's output has established about it, falling
// back to fallback when nothing has.
func (s *State) toneFor(ref event.BlockRef, fallback theme.Tone) theme.Tone {
	if t, ok := s.outcome[ref]; ok {
		return t
	}
	return fallback
}

func outputColor(status string) color.Color {
	switch status {
	case "ok":
		return theme.OK
	case "err":
		return theme.Deny
	default:
		return theme.TextDim
	}
}

func (s *State) applyToolCompleted(p event.ToolCompleted) {
	i := s.block(p.Block, tagTool, theme.ToneTool, "")

	if p.Status == "ok" {
		s.Blocks[i].Tone = s.toneFor(p.Block, theme.ToneTool)
	} else {
		s.Blocks[i].Tone = theme.ToneDeny
	}
	if p.Duration > 0 {
		s.Blocks[i].Meta = toolMeta(s.proposed[p.Block], duration(p.Duration))
	}

	if len(p.Changes) > 0 {
		rows := make([][]string, 0, len(p.Changes))
		for _, c := range p.Changes {
			summary := c.Summary
			if summary == "" {
				summary = signedChanges(c.Added, c.Removed)
			}
			rows = append(rows, []string{c.Op, c.Path, summary})
		}
		lines := make([]ui.Line, 0, len(rows))
		for _, row := range columnsAt(rows, toolStops, wideGap) {
			lines = append(lines, ui.Line{Text: row, Color: theme.TextDim})
		}
		s.Blocks[i].Lines = append(s.Blocks[i].Lines, lines...)
	}
	s.touch(p.Block)
}

// ------------------------------------------------------------------- rail

func (s *State) applyGoals(p event.GoalsUpdated) {
	s.Rail.Goals = nil
	for _, g := range p.Goals {
		s.Rail.Goals = append(s.Rail.Goals, ui.Goal{State: goalState(g.State), Text: g.Text})
	}
}

func goalState(s string) ui.GoalState {
	switch s {
	case "done":
		return ui.GoalDone
	case "active":
		return ui.GoalActive
	case "denied":
		return ui.GoalDenied
	case "waiting":
		return ui.GoalWaiting
	default:
		return ui.GoalPending
	}
}

func (s *State) applyLSP(p event.LSPUpdated) {
	if p.Server != "" {
		s.Rail.LSPServer = p.Server
	}
	s.Rail.Diagnostics = nil
	for _, d := range p.Diagnostics {
		at := ""
		if d.Line > 0 {
			at = fmt.Sprintf(":%d", d.Line)
		}
		s.Rail.Diagnostics = append(s.Rail.Diagnostics, ui.Diagnostic{
			Severity: severity(d.Severity), File: d.File, At: at,
		})
	}
}

func severity(s string) ui.Severity {
	switch s {
	case "err":
		return ui.SevErr
	case "warn":
		return ui.SevWarn
	case "hint":
		return ui.SevHint
	case "ok":
		return ui.SevOK
	default:
		return ui.SevPending
	}
}

func (s *State) applyUsage(p event.UsageUpdated) {
	s.usage = p
	s.refreshUsage()
}

// refreshUsage rebuilds the rail's cost section.
//
// It is called from both usage and fork events because either can arrive first
// and forking changes how cost should read. A fold that depended on the order
// would render a forked session differently on replay than it did live, which
// is exactly what the log is supposed to rule out.
func (s *State) refreshUsage() {
	p := s.usage
	limit := p.CtxLimit
	if limit == 0 {
		limit = s.ctxLimit
	}
	s.Rail.CtxPercent = contextPercent(p.CtxUsed, limit)
	s.Rail.CtxLabel = contextLabel(p.CtxUsed, limit)
	s.Rail.Cost = []ui.KV{
		s.turnCost(p.CostTurnUSD),
		{Key: "session", Value: money(p.CostSessionUSD)},
		{Key: "tok in / out", Value: tokens(p.TokensIn) + " / " + tokens(p.TokensOut)},
	}
}

// turnCost is the rail's first cost row. A forked session spent its turn in
// several places at once, so the design lists them side by side rather than
// summing them into a figure that describes none of them.
func (s *State) turnCost(turnUSD float64) ui.KV {
	if !s.forked || len(s.forks) == 0 {
		return ui.KV{Key: "this turn", Value: money(turnUSD)}
	}
	names := make([]string, 0, len(s.forks))
	amounts := make([]string, 0, len(s.forks))
	for _, f := range s.forks {
		names = append(names, f.Name)
		// Side by side the leading zero is noise; the "$" on the first is
		// enough to say these are all money.
		amounts = append(amounts, strings.TrimPrefix(fmt.Sprintf("%.2f", f.CostUSD), "0"))
	}
	return ui.KV{
		Key:   "fork " + strings.Join(names, " / "),
		Value: "$" + strings.Join(amounts, " "),
	}
}

func (s *State) applySandbox(p event.SandboxUpdated) {
	vmKey, sizeKey := "vm", "size"
	if p.Count > 1 {
		vmKey, sizeKey = "vms", "size"
	}
	rows := []ui.KV{{Key: vmKey, Value: p.VM}}
	if p.From != "" {
		rows = append(rows, ui.KV{Key: "from", Value: p.From})
	}
	rows = append(rows, ui.KV{Key: sizeKey, Value: p.Size})
	if p.From == "" {
		rows = append(rows, ui.KV{Key: "uptime", Value: uptime(p.Uptime)})
	}
	rows = append(rows, ui.KV{Key: "state", Value: p.State, Color: sandboxColor(p.State)})
	s.Rail.Sandbox = rows
}

func sandboxColor(state string) color.Color {
	switch {
	case strings.Contains(state, "suspend"):
		return theme.Human
	case strings.Contains(state, "running") && strings.Contains(state, "·"):
		return theme.Fork
	default:
		return theme.OK
	}
}

func (s *State) applyForks(p event.ForksUpdated) {
	i := s.block(p.Block, tagForks, theme.ToneFork, "identical start · independent loops")
	s.forked, s.forks = true, p.Forks
	s.refreshUsage()

	rows := make([][]string, 0, len(p.Forks))
	for _, f := range p.Forks {
		state := "✔ done"
		if f.State != "done" {
			state = fmt.Sprintf("⟳ turn %d", f.Turn)
		}
		tests := "…"
		if f.TestsRun > 0 {
			tests = fmt.Sprintf("%d/%d", f.TestsOK, f.TestsRun)
		}
		rows = append(rows, []string{
			f.Name, f.Approach, state,
			signedChanges(f.Added, f.Removed), tests, moneyCompact(f.CostUSD),
		})
	}

	lines := make([]ui.Line, 0, len(rows))
	for n, row := range columnsAt(rows, forkStops, wideGap) {
		colour := theme.TextNormal
		if p.Forks[n].State != "done" {
			colour = theme.TextSubtle
		}
		lines = append(lines, ui.Line{Text: row, Color: colour, Boxed: true})
	}
	s.Blocks[i].Lines = lines
	s.touch(p.Block)
}
