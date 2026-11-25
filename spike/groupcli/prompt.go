package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
)

//go:embed prompt_v1.txt
var promptV1 string

const PromptVersion = "v1"

type BuildOpts struct {
	MaxGroups    int
	TokenBudget  int // for the whole prompt
	Context      string
	ForceMaxBody int // >0 forces a per-hunk body cap (testing)
}

type Built struct {
	Prompt       string
	EstTokens    int
	BodyCap      int // lines kept per hunk; -1 means untruncated
	Truncated    int // hunks truncated
	DroppedLines int
}

// EstimateTokens is the cheap heuristic used before sending (bytes/4, rounded up).
// The spike compares it with the token counts providers report.
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }

func catalogLine(h *Hunk) string {
	f := h.File
	p := f.Path()
	if f.Status == "R" || f.Status == "C" {
		p = f.OldPath + " -> " + f.NewPath
	}
	return fmt.Sprintf("%s | %s | %s | %s | +%d -%d", h.ID, f.Status, p, h.Header, h.Add, h.Del)
}

func hunkBlock(h *Hunk, capLines int) (string, bool, int) {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s %s\n%s\n", h.ID, h.File.Path(), h.Header)
	body := h.Lines
	dropped := 0
	if capLines >= 0 && len(body) > capLines {
		dropped = len(body) - capLines
		body = body[:capLines]
	}
	for _, l := range body {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if dropped > 0 {
		fmt.Fprintf(&b, "[... %d more lines]\n", dropped)
	}
	return b.String(), dropped > 0, dropped
}

func nonce() string {
	var b [6]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func render(d *Diff, o BuildOpts, capLines int, n string) Built {
	var cat, hunks strings.Builder
	out := Built{BodyCap: capLines}
	for _, h := range d.Hunks {
		cat.WriteString(catalogLine(h))
		cat.WriteByte('\n')
		blk, tr, dr := hunkBlock(h, capLines)
		if tr {
			out.Truncated++
			out.DroppedLines += dr
		}
		hunks.WriteString(blk)
	}
	r := strings.NewReplacer("{{NONCE}}", n, "{{MAX_GROUPS}}", fmt.Sprint(o.MaxGroups),
		"{{CONTEXT}}", o.Context, "{{CATALOG}}", cat.String(), "{{HUNKS}}", hunks.String())
	out.Prompt = r.Replace(promptV1)
	out.EstTokens = EstimateTokens(out.Prompt)
	return out
}

// Build renders the prompt, shrinking hunk bodies (never headers or catalog entries)
// until it fits the token budget. Strategy: try untruncated, then binary-search the
// largest per-hunk line cap that fits; cap 0 keeps only catalog + headers.
func Build(d *Diff, o BuildOpts) Built {
	n := nonce()
	if o.ForceMaxBody > 0 {
		return render(d, o, o.ForceMaxBody, n)
	}
	full := render(d, o, -1, n)
	if o.TokenBudget <= 0 || full.EstTokens <= o.TokenBudget {
		return full
	}
	lo, hi := 0, 0
	for _, h := range d.Hunks {
		if len(h.Lines) > hi {
			hi = len(h.Lines)
		}
	}
	best := render(d, o, 0, n)
	for lo <= hi {
		mid := (lo + hi) / 2
		b := render(d, o, mid, n)
		if b.EstTokens <= o.TokenBudget {
			best, lo = b, mid+1
		} else {
			hi = mid - 1
		}
	}
	return best
}
