// INPUT: 冲突文件的三个阶段。
// OUTPUT: ReadMerge、MergeFile.Apply：三方内容与写回。
// POS: 冲突解决的数据层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"revtrail/internal/workspace"
	"strconv"
	"strings"
	"unicode/utf8"
)

type MergeStage struct {
	Mode, Blob string
	Text       string
}
type MergePart struct {
	Common, Left, Base, Right string
	Conflict                  bool
}
type MergeFile struct {
	Path              string
	Left, Base, Right MergeStage
	Parts             []MergePart
	Text              bool
	Edited            bool
	Working           string
	workingHash       [32]byte
	index             string
	workMode          os.FileMode
	leftStage         int
}

type mergeWork struct {
	text  []byte
	hash  [32]byte
	mode  os.FileMode
	large bool
}

func mergeWorking(root, path string, readText bool) (w mergeWork, err error) {
	name := filepath.Join(root, filepath.FromSlash(path))
	if !filepath.IsLocal(filepath.FromSlash(path)) {
		return w, fmt.Errorf("invalid conflict path: %s", path)
	}
	for dir := filepath.Dir(name); dir != filepath.Clean(root); dir = filepath.Dir(dir) {
		info, e := os.Lstat(dir)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return w, e
		}
		if !info.IsDir() {
			return w, fmt.Errorf("conflict parent is not a directory: %s", dir)
		}
	}
	fi, err := os.Lstat(name)
	if os.IsNotExist(err) {
		return w, nil
	}
	if err != nil {
		return w, err
	}
	w.mode = fi.Mode()
	if w.mode&os.ModeSymlink != 0 {
		target, err := os.Readlink(name)
		w.hash = sha256.Sum256([]byte(target))
		return w, err
	}
	if !w.mode.IsRegular() {
		return w, fmt.Errorf("use Git to resolve this directory or submodule: %s", path)
	}
	w.large = fi.Size() > workspace.PlainLimit
	if readText && !w.large {
		w.text, err = os.ReadFile(name)
		w.hash = sha256.Sum256(w.text)
		return w, err
	}
	f, err := os.Open(name)
	if err != nil {
		return w, err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	copy(w.hash[:], h.Sum(nil))
	return w, err
}

func ReadMerge(ctx context.Context, root, path string) (*MergeFile, error) {
	raw, err := Command(ctx, root, "ls-files", "-u", "-z", "--", path)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("this file no longer has a Git conflict")
	}
	m := &MergeFile{Path: path, index: string(raw), leftStage: 2}
	var stages [4]MergeStage
	for entry := range strings.SplitSeq(strings.TrimSuffix(m.index, "\x00"), "\x00") {
		meta, p, _ := strings.Cut(entry, "\t")
		f := strings.Fields(meta)
		if len(f) != 3 || p != path {
			return nil, errors.New("unexpected Git conflict entry")
		}
		n, e := strconv.Atoi(f[2])
		if e != nil || n < 1 || n > 3 {
			return nil, errors.New("invalid Git stage")
		}
		if f[0] == "160000" {
			return nil, fmt.Errorf("resolve submodule %s in Git", path)
		}
		stages[n] = MergeStage{Mode: f[0], Blob: f[1]}
	}
	// A rebase replays our commits onto theirs: stage 3 is the side being
	// rebased, so it shows on the left.
	if OperationState(ctx, root) == OpRebasing {
		m.leftStage = 3
	}
	work, err := mergeWorking(root, path, true)
	if err != nil {
		return nil, err
	}
	m.workMode, m.workingHash = work.mode, work.hash
	m.Left, m.Base, m.Right = stages[m.leftStage], stages[1], stages[5-m.leftStage]
	m.Text = m.Left.Mode == m.Right.Mode && strings.HasPrefix(m.Left.Mode, "100") && !work.large && (work.mode == 0 || work.mode.IsRegular()) && utf8.Valid(work.text) && bytes.IndexByte(work.text, 0) < 0
	if !m.Text {
		return m, nil
	}
	for _, s := range []*MergeStage{&m.Left, &m.Base, &m.Right} {
		if s.Blob == "" {
			continue
		}
		if !strings.HasPrefix(s.Mode, "100") {
			m.Text = false
			return m, nil
		}
		size, err := Command(ctx, root, "cat-file", "-s", s.Blob)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(size)), 10, 64)
		if err != nil {
			return nil, err
		}
		if n > workspace.PlainLimit {
			m.Text = false
			return m, nil
		}
		b, err := command(ctx, root, "cat-file", "--filters", "--path="+path, s.Blob).Output()
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
			m.Text = false
			return m, nil
		}
		s.Text = string(b)
	}
	m.Working = string(work.text)
	m.Parts, err = mergeParts(ctx, m.Left.Text, m.Base.Text, m.Right.Text)
	if err != nil {
		return nil, err
	}
	if saved, e := Command(ctx, root, "rev-parse", "--verify", "--quiet", "AUTO_MERGE:"+path); e == nil {
		if original, e := command(ctx, root, "cat-file", "--filters", "--path="+path, strings.TrimSpace(string(saved))).Output(); e == nil {
			m.Edited = !bytes.Equal(original, work.text)
		}
	} else {
		markers := 0
		for line := range strings.SplitSeq(m.Working, "\n") {
			if strings.HasPrefix(line, "<<<<<<<") {
				markers++
			}
		}
		for _, part := range m.Parts {
			if part.Conflict {
				markers--
			}
		}
		m.Edited = work.mode != 0 && markers != 0
	}
	return m, nil
}

func mergeParts(ctx context.Context, left, base, right string) ([]MergePart, error) {
	dir, err := os.MkdirTemp("", "revtrail-merge-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	args := []string{"merge-file", "-p", "--diff3", "-L", "left", "-L", "base", "-L", "right"}
	size := 7
	used := map[int]bool{}
	for i, s := range []string{left, base, right} {
		for line := range strings.SplitSeq(s, "\n") {
			if len(line) > 0 && strings.ContainsRune("<|=>", rune(line[0])) {
				n := 1
				for n < len(line) && line[n] == line[0] {
					n++
				}
				used[n] = true
			}
		}
		if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(i)), []byte(s), 0600); err != nil {
			return nil, err
		}
	}
	for used[size] {
		size++
	}
	args = append(args, "--marker-size", strconv.Itoa(size), "0", "1", "2")
	out, err := command(ctx, dir, args...).Output()
	if err != nil {
		var e *exec.ExitError
		if !errors.As(err, &e) || e.ExitCode() < 1 || e.ExitCode() > 127 {
			return nil, fmt.Errorf("git merge-file: %w", err)
		}
	}
	var parts []MergePart
	var part MergePart
	var buf strings.Builder
	state := 0
	l, b, r, sep := strings.Repeat("<", size)+" left", strings.Repeat("|", size)+" base", strings.Repeat(">", size)+" right", strings.Repeat("=", size)
	for line := range strings.SplitAfterSeq(string(out), "\n") {
		switch strings.TrimRight(line, "\r\n") {
		case l:
			if buf.Len() > 0 {
				parts = append(parts, MergePart{Common: buf.String()})
			}
			part = MergePart{Conflict: true}
			state = 1
		case b:
			part.Left = buf.String()
			state = 2
		case sep:
			part.Base = buf.String()
			state = 3
		case r:
			part.Right = buf.String()
			parts = append(parts, part)
			state = 0
		default:
			buf.WriteString(line)
			continue
		}
		buf.Reset()
	}
	if state != 0 {
		return nil, errors.New("incomplete merge result")
	}
	if buf.Len() > 0 {
		parts = append(parts, MergePart{Common: buf.String()})
	}
	if len(parts) > 0 && parts[len(parts)-1].Conflict {
		last := &parts[len(parts)-1]
		for _, pair := range []struct {
			source   string
			fragment *string
		}{{left, &last.Left}, {base, &last.Base}, {right, &last.Right}} {
			if !strings.HasSuffix(pair.source, "\n") {
				*pair.fragment = strings.TrimSuffix(*pair.fragment, "\n")
			}
		}
	}
	return parts, nil
}

// Apply resolves only this path. The stages and working copy must still match what the merge window opened.
func (m *MergeFile) Apply(ctx context.Context, root, choice, result string) error {
	index, err := lockIndexCopy(ctx, root)
	if err != nil {
		return err
	}
	hold := true
	defer func() {
		if hold {
			os.Remove(index.file)
		}
	}()
	raw, err := index.run(ctx, "", "ls-files", "-u", "-z", "--", m.Path)
	if err != nil {
		return err
	}
	work, err := mergeWorking(root, m.Path, false)
	if err != nil {
		return err
	}
	if string(raw) != m.index || work.mode != m.workMode || work.hash != m.workingHash {
		return errors.New("file or Git conflict changed; reopen the merge window")
	}
	side := m.Left
	switch choice {
	case "left":
	case "right":
		side = m.Right
	case "result":
		if !m.Text {
			return errors.New("this conflict requires a whole-file choice")
		}
		cmd := index.command(ctx, "hash-object", "-w", "--path="+m.Path, "--stdin")
		cmd.Stdin = strings.NewReader(result)
		hash, err := cmd.Output()
		if err != nil {
			return err
		}
		side.Blob = strings.TrimSpace(string(hash))
	default:
		return errors.New("unknown resolution")
	}
	if side.Blob == "" {
		if _, err := index.run(ctx, "", "update-index", "--force-remove", "--", m.Path); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(root, m.Path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		if _, err := index.run(ctx, "", "update-index", "--add", "--cacheinfo", side.Mode, side.Blob, m.Path); err != nil {
			return err
		}
		if _, err := index.run(ctx, "", "checkout-index", "-f", "--", m.Path); err != nil {
			return err
		}
	}
	hold = false
	if err := os.Rename(index.file, strings.TrimSuffix(index.file, ".lock")); err != nil {
		return fmt.Errorf("could not publish resolution; index retained at %s: %w", index.file, err)
	}
	return nil
}
