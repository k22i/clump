package tui

// Posting a review: P on a diff opened with --pr turns the diff's stored
// comments into one GitHub pull request review — line comments inline on
// their line, file comments on the file, group comments in the body — and
// posts it with the event picked here (comment, approve or request changes;
// internal/forge). Everything goes through the gh CLI, like --pr itself:
// no tokens are stored, the review posts as the account gh is logged into.

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/forge"
)

// submitState is the event picker: one of the three REST review events.
type submitState struct {
	items  []menuItem
	cursor int
	title  string
}

// submitMsg reports a finished forge.Submit.
type submitMsg struct {
	event string
	url   string
	err   error
}

// openSubmit is P: show the event picker over the pull request's comments.
func (m *Model) openSubmit() tea.Cmd {
	if m.o.PR == "" {
		m.setFlash("P posts a review: open a pull request first (clump --pr SPEC)", true)
		return nil
	}
	if m.doc == nil || m.doc.L == nil || len(m.doc.L.Diff.Hunks) == 0 {
		return nil
	}
	placed := m.reportComments()
	if len(placed) == 0 {
		m.setFlash("no comments to submit: write them with C", true)
		return nil
	}
	r := forge.Build(forge.EventComment, "", placed, m.doc.L.Diff, m.groupTitle())
	m.submit = submitState{
		items: []menuItem{
			{name: "comment", label: "Comment — post the review comments"},
			{name: "approve", label: "Approve — post them and approve the pull request"},
			{name: "request-changes", label: "Request changes — post them and request changes"},
		},
		title: "Post review of " + m.doc.L.Ref + ": " + r.Summary(),
	}
	m.ov = ovSubmit
	return nil
}

// keySubmit drives the event picker.
func (m *Model) keySubmit(key string) tea.Cmd {
	s := &m.submit
	switch key {
	case "j", "down":
		s.cursor = min(s.cursor+1, len(s.items)-1)
	case "k", "up":
		s.cursor = max(s.cursor-1, 0)
	case "esc", "q", "P":
		m.ov = ovNone
		return nil
	case "enter":
		m.ov = ovNone
		if s.cursor >= 0 && s.cursor < len(s.items) {
			return m.postReview(s.items[s.cursor].name)
		}
	}
	return nil
}

// postReview submits one event over the diff's placed comments.
func (m *Model) postReview(name string) tea.Cmd {
	event, err := forge.ParseEvent(name)
	if err != nil {
		m.setFlash(err.Error(), true)
		return nil
	}
	placed := m.reportComments()
	r := forge.Build(event, "", placed, m.doc.L.Diff, m.groupTitle())
	if r.Counts.Total() == 0 {
		m.setFlash("no comments to submit", true)
		return nil
	}
	m.setFlash("submitting "+event.String()+" review…", false)
	ctx, pr, repo := m.ctx, m.o.PR, m.o.Repo
	return tea.Batch(m.status("working", "submitting "+event.String()+" review"), func() tea.Msg {
		res, err := forge.Submit(ctx, repo, pr, r)
		msg := submitMsg{event: event.String(), err: err}
		if err == nil {
			msg.url = res.URL
		}
		return msg
	})
}

// onSubmitMsg reports the forge result in the flash.
func (m *Model) onSubmitMsg(msg submitMsg) tea.Cmd {
	if msg.err != nil {
		m.setFlash("review not submitted: "+msg.err.Error(), true)
		return m.status("error", msg.err.Error())
	}
	where := msg.event + " review submitted"
	if msg.url != "" {
		m.setFlash(where+": "+msg.url, false)
	} else {
		m.setFlash(where, false)
	}
	return m.status("done", where)
}

// groupTitle names a group of the on-screen grouping for the review body.
func (m *Model) groupTitle() func(id string) string {
	g := m.groups()
	return func(id string) string {
		for _, grp := range g {
			if grp.ID == id && strings.TrimSpace(grp.Title) != "" {
				return grp.Title
			}
		}
		return ""
	}
}
