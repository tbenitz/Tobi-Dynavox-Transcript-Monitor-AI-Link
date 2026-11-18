package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	quick   = "Quick replies"
	words   = "Next words"
	hybrid  = "Hybrid"
	maxTail = 8192
)

type suggestion struct {
	Text string
	Word bool
}
type predictions struct {
	Replies []string `json:"replies"`
	Words   []string `json:"words"`
}

var sentence = regexp.MustCompile(`[^.!?\r\n]+[.!?]*`)
var wordToken = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{M}\p{N}'’\-]*`)
var choice = regexp.MustCompile(`(?i)(?:like|prefer|want|choose|rather have)\s+([\p{L}][\p{L}'’-]*(?:\s+[\p{L}][\p{L}'’-]*){0,2})\s+or\s+([\p{L}][\p{L}'’-]*(?:\s+[\p{L}][\p{L}'’-]*){0,2})\s*[?.!]?$`)

func recent(s, unit string, n int) string {
	s = strings.TrimSpace(strings.ToValidUTF8(strings.TrimPrefix(s, "\uFEFF"), ""))
	if n < 1 || s == "" {
		return ""
	}
	if unit == "words" {
		a := strings.Fields(s)
		return strings.Join(a[max(0, len(a)-n):], " ")
	}
	var a []string
	for _, p := range sentence.FindAllString(s, -1) {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			a = append(a, p)
		}
	}
	return strings.Join(a[max(0, len(a)-n):], " ")
}
func upperFirst(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+utf8.RuneLen(r):]
	}
	return s
}
func lastPrefix(draft string) string {
	i := strings.LastIndexFunc(draft, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r) && r != '\'' && r != '’' && r != '-'
	})
	if i < 0 {
		return draft
	}
	_, n := utf8.DecodeRuneInString(draft[i:])
	return draft[i+n:]
}
func offline(heard, draft, mode string) []suggestion {
	line := recent(heard, "sentences", 1)
	q := strings.ToLower(line)
	replies := []string{"I’m listening.", "Give me a moment.", "Can you repeat that?", "I have something to add.", "Thanks for telling me."}
	if m := choice.FindStringSubmatch(line); len(m) == 3 {
		replies = []string{upperFirst(m[1]) + ", please.", upperFirst(m[2]) + ", please.", "Neither, thanks.", "Could you say the choices again?", "Give me a moment."}
	} else if strings.Contains(q, "how are you") || strings.Contains(q, "how’s it going") {
		replies = []string{"I’m doing well.", "I’m okay.", "I’m having a hard day.", "How are you?", "Give me a moment."}
	} else if strings.Contains(q, "thank") {
		replies = []string{"You’re welcome.", "Of course.", "Thank you, too.", "I appreciate that.", "Give me a moment."}
	} else if strings.Contains(q, "your name") && strings.Contains(q, "?") {
		replies = []string{"My name is…", "Let me type it.", "Give me a moment.", "Could you repeat that?", "I’d rather not say."}
	} else if strings.Contains(q, "?") {
		replies = []string{"Yes, please.", "No, thank you.", "I’m not sure.", "Can you repeat that?", "Give me a moment."}
	} else if strings.Contains(q, "hello") || strings.HasPrefix(q, "hi ") || q == "hi" || q == "hi!" {
		replies = []string{"Hi!", "Nice to see you.", "How are you?", "Thanks for coming.", "Give me a moment."}
	}
	frequent := strings.Fields("I you the and to a it is that can yes no please thanks help want need would like have will do go get good fine okay more some now later today tomorrow again because but with about here there water coffee tea food rest stop wait time home talk tell say see think feel know understand mean not ready tired happy hungry thirsty better sure thing moment second could should how what when where why which who we they this me my your for in on at of be am are was were did very really something anything everything right left open close turn up down next soon much little love thank")
	next := []string{}
	normalized := strings.ToLower(strings.TrimSpace(draft))
	patterns := []struct{ suffix, options string }{
		{"would like", "some to water more help coffee tea a"},
		{"can you", "help repeat wait tell please move get"},
		{"thank", "you them him her everyone"},
		{"i am", "okay fine tired ready happy hungry thirsty"},
		{"i need", "help water a some to more rest"},
		{"i want", "to some water a more help"},
		{"i", "am need want can would think feel have"},
		{"do you", "want know think have need like"},
	}
	prefix := lastPrefix(draft)
	if prefix != "" {
		normalized = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(draft), strings.ToLower(prefix)))
	}
	for _, p := range patterns {
		if normalized == p.suffix || strings.HasSuffix(normalized, " "+p.suffix) {
			next = append(next, strings.Fields(p.options)...)
			break
		}
	}
	if len(next) == 0 && draft != "" {
		last := wordToken.FindAllString(normalized, -1)
		if len(last) > 0 {
			for _, phrase := range strings.Split(heard, "\n") {
				a := wordToken.FindAllString(phrase, -1)
				for i := len(a) - 2; i >= 0; i-- {
					if strings.EqualFold(a[i], last[len(last)-1]) {
						next = append(next, a[i+1])
					}
				}
			}
		}
	}
	stop := " i you is it a an the to and or are do did does would could should can will of at in on for your my what when where who why how that this have we us they has he she be with like want prefer choose "
	context := wordToken.FindAllString(strings.ToLower(line), -1)
	slices.Reverse(context)
	for _, w := range context {
		if len(w) > 2 && !strings.Contains(stop, " "+w+" ") {
			next = append(next, w)
		}
	}
	next = append(next, frequent...)
	var actual []string
	for _, w := range next {
		if prefix != "" && (!strings.HasPrefix(strings.ToLower(w), strings.ToLower(prefix)) || strings.EqualFold(w, prefix)) {
			continue
		}
		if !slices.ContainsFunc(actual, func(a string) bool { return strings.EqualFold(a, w) }) {
			actual = append(actual, w)
		}
	}
	return combine(predictions{}, predictions{replies, actual}, mode)
}
func combine(ai, fallback predictions, mode string) []suggestion {
	r, w := 2, 3
	if mode == quick {
		r, w = 5, 0
	}
	if mode == words {
		r, w = 0, 5
	}
	result := make([]suggestion, 0, r+w)
	add := func(primary, secondary []string, count int, word bool) {
		if count <= 0 {
			return
		}
		n := 0
		for _, stream := range [][]string{primary, secondary} {
			for _, v := range stream {
				if slices.ContainsFunc(result, func(s suggestion) bool { return strings.EqualFold(s.Text, v) }) {
					continue
				}
				result = append(result, suggestion{v, word})
				n++
				if n >= count {
					return
				}
			}
		}
	}
	add(ai.Replies, fallback.Replies, r, false)
	add(ai.Words, fallback.Words, w, true)
	return result
}
func mergeAI(p predictions, fallback []suggestion, mode string) []suggestion {
	var f predictions
	for _, s := range fallback {
		if s.Word {
			f.Words = append(f.Words, s.Text)
		} else {
			f.Replies = append(f.Replies, s.Text)
		}
	}
	return combine(p, f, mode)
}
func insertion(s suggestion, draft string, space, punct bool) string {
	v := s.Text
	if s.Word {
		prefix := lastPrefix(draft)
		if prefix != "" {
			r := []rune(v)
			n := utf8.RuneCountInString(prefix)
			if n >= len(r) || !strings.EqualFold(string(r[:n]), prefix) {
				return ""
			}
			v = string(r[n:])
		}
	} else if punct && !strings.ContainsRune(".?!…", []rune(v)[len([]rune(v))-1]) {
		v += "."
	}
	if space && s.Word {
		v += " "
	}
	return v
}

type follower struct {
	protected bool
	ready     bool
	blockedAt int64
	anchor    []byte
	info      os.FileInfo
}

func (f *follower) resume(path string) {
	*f = follower{protected: true}
	file, e := openTranscript(path)
	if e == nil {
		defer file.Close()
		if info, e := file.Stat(); e == nil {
			_ = f.baseline(file, info)
		}
	}
}
func (f *follower) baseline(file *os.File, info os.FileInfo) error {
	end := info.Size()
	a := make([]byte, min(end, 128))
	if _, e := file.ReadAt(a, end-int64(len(a))); e != nil && len(a) > 0 {
		return e
	}
	f.ready, f.blockedAt, f.anchor, f.info = true, end, a, info
	return nil
}
func (f *follower) read(path string) (string, error) {
	file, e := openTranscript(path)
	if e != nil {
		return "", e
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil {
		return "", e
	}
	end := info.Size()
	floor := int64(0)
	if f.protected {
		if !f.ready || !os.SameFile(f.info, info) || end < f.blockedAt {
			return "", f.baseline(file, info)
		}
		a := make([]byte, len(f.anchor))
		if len(a) > 0 {
			if n, e := file.ReadAt(a, f.blockedAt-int64(len(a))); e != nil || n != len(a) {
				return "", e
			}
			if !bytes.Equal(a, f.anchor) {
				return "", f.baseline(file, info)
			}
		}
		floor = f.blockedAt
	}
	start := max(floor, end-maxTail)
	a, e := io.ReadAll(io.NewSectionReader(file, start, end-start))
	if e != nil {
		return "", e
	}
	if start > floor {
		if i := bytes.IndexByte(a, '\n'); i >= 0 {
			a = a[i+1:]
		}
	}
	for len(a) > 0 && a[0]&0xC0 == 0x80 {
		a = a[1:]
	}
	return string(a), nil
}

func parseHotkey(s string) (uint32, uint32, error) {
	a := strings.Split(strings.ToUpper(strings.ReplaceAll(s, " ", "")), "+")
	var mods uint32
	for _, p := range a[:len(a)-1] {
		m := map[string]uint32{"ALT": 1, "CTRL": 2, "CONTROL": 2, "SHIFT": 4, "WIN": 8}[p]
		if m == 0 || mods&m != 0 {
			return 0, 0, fmt.Errorf("Use Ctrl, Alt, Shift, or Win once per shortcut")
		}
		mods |= m
	}
	key := a[len(a)-1]
	vk := map[string]uint32{"SPACE": 32, "BACKSPACE": 8, "INSERT": 45, "DELETE": 46, "HOME": 36, "END": 35, "PAGEUP": 33, "PAGEDOWN": 34}[key]
	isFunction := false
	if strings.HasPrefix(key, "F") {
		if n, e := strconv.Atoi(key[1:]); e == nil && n >= 1 && n <= 24 {
			vk = uint32(111 + n)
			isFunction = true
		}
	}
	if vk == 0 && len(key) == 1 && ((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= '0' && key[0] <= '9')) {
		vk = uint32(key[0])
	}
	if vk == 0 || mods == 0 && !isFunction {
		return 0, 0, fmt.Errorf("Choose F1–F24 or a shortcut such as Ctrl+Space")
	}
	return mods, vk, nil
}
func mayOpen(now, closed time.Time, cooldown int) bool {
	return now.Sub(closed) >= time.Duration(cooldown)*time.Millisecond
}
