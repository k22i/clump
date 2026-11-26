package lazygit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func PushCommand(then string) string {
	if then == "" {
		return "clump push"
	}
	return "clump push --then '" + strings.ReplaceAll(then, "'", `'\''`) + "'"
}

func Snippet(then string) string {
	return "git:\n" +
		"  diffRenderers:\n" +
		"    - command: " + yamlQuote(PushCommand(then)) + "\n" +
		"      name: clump\n"
}

func ConfigPath() (string, error) {
	if lg := os.Getenv("LG_CONFIG_FILE"); lg != "" {
		if first := strings.Split(lg, ",")[0]; first != "" {
			return first, nil
		}
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "lazygit", "config.yml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		p := filepath.Join(home, "Library", "Application Support", "lazygit", "config.yml")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return filepath.Join(home, ".config", "lazygit", "config.yml"), nil
}

type InstallOpts struct {
	Path  string
	Then  string
	Write bool
	In    io.Reader
	Out   io.Writer
}

func Install(o InstallOpts) error {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	content, exists, err := readConfig(o.Path)
	if err != nil {
		return err
	}
	e := planEdit(o.Path, content, o.Then)
	if _, err := io.WriteString(out, e.out); err != nil {
		return err
	}
	if !e.changed || !o.Write {
		return nil
	}

	if _, err := fmt.Fprintf(out, "Write this to %s? [y/N] ", o.Path); err != nil {
		return err
	}
	in := o.In
	if in == nil {
		in = strings.NewReader("")
	}
	answer, err := readLine(in)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if !isYes(answer) {
		_, err := fmt.Fprintln(out, "not written")
		return err
	}

	perm := fs.FileMode(0o644)
	backup := ""
	if exists {
		info, err := os.Stat(o.Path)
		if err != nil {
			return err
		}
		perm = info.Mode().Perm()
		backup = o.Path + ".clump-backup-" + time.Now().Format("20060102-150405")
		if err := copyFile(o.Path, backup, perm); err != nil {
			return err
		}
	}
	if dir := filepath.Dir(o.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(o.Path, []byte(e.content), perm); err != nil {
		return err
	}
	if err := os.Chmod(o.Path, perm); err != nil {
		return err
	}
	if backup != "" {
		_, err = fmt.Fprintf(out, "wrote %s (backup: %s)\n", o.Path, backup)
	} else {
		_, err = fmt.Fprintf(out, "wrote %s\n", o.Path)
	}
	return err
}

type edit struct {
	content string
	out     string
	changed bool
}

var (
	gitKeyRE    = regexp.MustCompile(`^git:\s*(#.*)?$`)
	renderersRE = regexp.MustCompile(`^\s*diffRenderers:\s*(#.*)?$`)
	pagingRE    = regexp.MustCompile(`^\s*paging:\s*(#.*)?$`)
)

func planEdit(path, content, then string) edit {
	snippet := Snippet(then)
	if content == "" {
		return edit{
			content: snippet,
			out:     "lazygit config " + path + " does not exist; it will contain:\n" + snippet,
			changed: true,
		}
	}
	lines := strings.Split(content, "\n")

	gi := -1
	for i, l := range lines {
		if !strings.HasPrefix(l, "git:") {
			continue
		}
		if !gitKeyRE.MatchString(l) {
			return refuse(path, content, "git: has an inline value", snippet)
		}
		gi = i
		break
	}
	if gi < 0 {
		base := content
		if !strings.HasSuffix(base, "\n") {
			base += "\n"
		}
		return edit{
			content: base + snippet,
			out:     "add this to " + path + ":\n" + snippet,
			changed: true,
		}
	}

	end := len(lines)
	for j := gi + 1; j < len(lines); j++ {
		l := lines[j]
		if l == "" {
			continue
		}
		if l[0] != ' ' && l[0] != '\t' && l[0] != '#' {
			end = j
			break
		}
	}
	block := lines[gi+1 : end]

	for _, l := range block {
		if hasAnchorOrAlias(l) {
			return refuse(path, content, "the git block uses a YAML anchor or alias", snippet)
		}
	}
	if hasLegacyPager(block) {
		return refuse(path, content, "the git block sets git.paging.pager", snippet)
	}

	dri := -1
	for idx, l := range block {
		if !strings.HasPrefix(strings.TrimLeft(l, " \t"), "diffRenderers:") {
			continue
		}
		if !renderersRE.MatchString(l) {
			return refuse(path, content, "diffRenderers has an inline value", snippet)
		}
		dri = idx
		break
	}
	if dri < 0 {
		ind := "  "
		for _, l := range block {
			if l == "" {
				continue
			}
			ind = leadingWS(l)
			break
		}
		ins := rendererLines(ind, then)
		return edit{
			content: insertLines(lines, gi+1, ins),
			out:     "insert under git: in " + path + ":\n" + strings.Join(ins, "\n") + "\n",
			changed: true,
		}
	}

	driInd := indentOf(block[dri])
	start := -1
	for k := dri + 1; k < len(block); k++ {
		if block[k] == "" {
			continue
		}
		if indentOf(block[k]) > driInd && strings.HasPrefix(strings.TrimLeft(block[k], " \t"), "- ") {
			start = k
		}
		break
	}
	if start < 0 {
		return refuse(path, content, "diffRenderers is not a block list", snippet)
	}

	regionEnd := start
	for k := start; k < len(block); k++ {
		if block[k] != "" && indentOf(block[k]) <= driInd {
			break
		}
		regionEnd = k + 1
	}
	for _, l := range block[start:regionEnd] {
		if strings.Contains(l, "clump push") {
			return edit{content: content, out: "clump push is already configured in " + path + "\n"}
		}
	}

	itemInd := indentOf(block[start])
	itemEnd := start + 1
	for k := start + 1; k < len(block); k++ {
		if block[k] == "" {
			itemEnd = k + 1
			continue
		}
		if indentOf(block[k]) <= itemInd {
			break
		}
		itemEnd = k + 1
	}
	typ, colorArg, cmdLine := "", "", -1
	for idx, l := range block[start:itemEnd] {
		t := strings.TrimSpace(l)
		t = strings.TrimSpace(strings.TrimPrefix(t, "- "))
		switch {
		case typ == "" && strings.HasPrefix(t, "type:"):
			typ = scalarValue(t[len("type:"):])
		case colorArg == "" && strings.HasPrefix(t, "colorArg:"):
			colorArg = scalarValue(t[len("colorArg:"):])
		case cmdLine < 0 && strings.HasPrefix(t, "command:"):
			cmdLine = start + idx
		}
	}
	if colorArg == "never" {
		return refuse(path, content, "the first diff renderer sets colorArg: never", snippet)
	}
	if (typ != "" && typ != "stdinFilter") || cmdLine < 0 {
		ins := rendererItem(leadingWS(block[start]), then)
		abs := gi + 1 + start
		var b strings.Builder
		b.WriteString("change the diff renderer in " + path + ":\n")
		for _, l := range ins {
			b.WriteString("+" + l + "\n")
		}
		return edit{content: insertLines(lines, abs, ins), out: b.String(), changed: true}
	}

	prefix, value, rest, ok := splitCommand(lines[gi+1+cmdLine])
	if !ok {
		return refuse(path, content, "the first diff renderer has no command", snippet)
	}
	newLine := prefix + yamlQuote(PushCommand(unquote(value))) + rest
	newLines := append([]string(nil), lines...)
	newLines[gi+1+cmdLine] = newLine
	return edit{
		content: strings.Join(newLines, "\n"),
		out: "change the diff renderer in " + path + ":\n" +
			"-" + lines[gi+1+cmdLine] + "\n" +
			"+" + newLine + "\n",
		changed: true,
	}
}

func rendererItem(ind, then string) []string {
	return []string{
		ind + "- command: " + yamlQuote(PushCommand(then)),
		ind + "  name: clump",
	}
}

func rendererLines(ind, then string) []string {
	item := rendererItem(ind+"  ", then)
	return []string{ind + "diffRenderers:", item[0], item[1]}
}

func insertLines(lines []string, at int, ins []string) string {
	out := make([]string, 0, len(lines)+len(ins))
	out = append(out, lines[:at]...)
	out = append(out, ins...)
	out = append(out, lines[at:]...)
	return strings.Join(out, "\n")
}

func refuse(path, content, reason, snippet string) edit {
	return edit{
		content: content,
		out:     "cannot edit " + path + " automatically: " + reason + "; add this by hand:\n" + snippet,
	}
}

func readConfig(path string) (content string, exists bool, err error) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		return string(b), true, nil
	case errors.Is(err, fs.ErrNotExist):
		return "", false, nil
	default:
		return "", false, err
	}
}

func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, err
}

func isYes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

func copyFile(src, dst string, perm fs.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, perm); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}

func yamlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

func indentOf(l string) int {
	n := 0
	for n < len(l) && (l[n] == ' ' || l[n] == '\t') {
		n++
	}
	return n
}

func leadingWS(l string) string {
	return l[:indentOf(l)]
}

func hasAnchorOrAlias(l string) bool {
	code := l
	for i := 1; i < len(code); i++ {
		if code[i] == '#' && (code[i-1] == ' ' || code[i-1] == '\t') {
			code = code[:i]
			break
		}
	}
	for i := 0; i < len(code); i++ {
		if c := code[i]; c == '&' || c == '*' {
			if i > 0 {
				switch code[i-1] {
				case ' ', '\t', ':', '-', ',', '[', '{':
				default:
					continue
				}
			}
			if i+1 < len(code) && isNameByte(code[i+1]) {
				return true
			}
		}
	}
	return false
}

func isNameByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func hasLegacyPager(block []string) bool {
	for idx, l := range block {
		if !pagingRE.MatchString(l) {
			continue
		}
		ind := indentOf(l)
		for k := idx + 1; k < len(block); k++ {
			cl := block[k]
			if cl == "" {
				continue
			}
			if indentOf(cl) <= ind {
				break
			}
			if strings.HasPrefix(strings.TrimSpace(cl), "pager:") {
				return true
			}
		}
	}
	return false
}

func scalarValue(s string) string {
	v, _ := splitScalar(strings.TrimSpace(s))
	return unquote(v)
}

func splitScalar(s string) (value, rest string) {
	if s == "" {
		return "", ""
	}
	switch s[0] {
	case '"':
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' {
				i++
				continue
			}
			if s[i] == '"' {
				return s[:i+1], s[i+1:]
			}
		}
		return s, ""
	case '\'':
		for i := 1; i < len(s); i++ {
			if s[i] != '\'' {
				continue
			}
			if i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			return s[:i+1], s[i+1:]
		}
		return s, ""
	default:
		for i := 0; i < len(s); i++ {
			if s[i] == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
				return strings.TrimRight(s[:i], " \t"), s[i-1:]
			}
		}
		return strings.TrimRight(s, " \t"), ""
	}
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		var b strings.Builder
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) {
				switch inner[i+1] {
				case '"', '\\':
					b.WriteByte(inner[i+1])
					i++
					continue
				}
			}
			b.WriteByte(inner[i])
		}
		return b.String()
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}

func splitCommand(l string) (prefix, value, rest string, ok bool) {
	idx := strings.Index(l, "command:")
	if idx < 0 {
		return "", "", "", false
	}
	end := idx + len("command:")
	after := l[end:]
	trimmed := strings.TrimLeft(after, " \t")
	if trimmed == "" {
		return "", "", "", false
	}
	sp := after[:len(after)-len(trimmed)]
	value, rest = splitScalar(trimmed)
	return l[:end] + sp, value, rest, true
}
