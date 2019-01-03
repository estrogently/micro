package action

import (
	"strings"

	"github.com/zyedidia/micro/cmd/micro/display"
	"github.com/zyedidia/micro/cmd/micro/info"
	"github.com/zyedidia/tcell"
)

type InfoKeyAction func(*InfoHandler)

type InfoHandler struct {
	*BufHandler
	*info.InfoBuf
}

func NewInfoHandler(ib *info.InfoBuf, w display.Window) *InfoHandler {
	ih := new(InfoHandler)
	ih.InfoBuf = ib
	ih.BufHandler = NewBufHandler(ib.Buffer, w)

	return ih
}

func (h *InfoHandler) HandleEvent(event tcell.Event) {
	switch e := event.(type) {
	case *tcell.EventKey:
		ke := KeyEvent{
			code: e.Key(),
			mod:  e.Modifiers(),
			r:    e.Rune(),
		}

		done := h.DoKeyEvent(ke)
		if !done && e.Key() == tcell.KeyRune {
			h.DoRuneInsert(e.Rune())
			done = true
		}
		if done && h.HasPrompt {
			resp := strings.TrimSpace(string(h.LineBytes(0)))
			hist := h.History[h.PromptType]
			hist[h.HistoryNum] = resp
			if h.EventCallback != nil {
				h.EventCallback(resp)
			}
		}
	case *tcell.EventMouse:
		h.BufHandler.HandleEvent(event)
	}
}

func (h *InfoHandler) DoKeyEvent(e KeyEvent) bool {
	done := false
	if action, ok := BufKeyBindings[e]; ok {
		estr := BufKeyStrings[e]
		for _, s := range InfoNones {
			if s == estr {
				return false
			}
		}
		for s, a := range InfoOverrides {
			if s == estr {
				done = true
				a(h)
				break
			}
		}
		if !done {
			done = action(h.BufHandler)
		}
	}
	return done
}

// InfoNones is a list of actions that should have no effect when executed
// by an infohandler
var InfoNones = []string{
	"Save",
	"SaveAll",
	"SaveAs",
	"Find",
	"FindNext",
	"FindPrevious",
	"Center",
	"DuplicateLine",
	"MoveLinesUp",
	"MoveLinesDown",
	"OpenFile",
	"Start",
	"End",
	"PageUp",
	"PageDown",
	"SelectPageUp",
	"SelectPageDown",
	"HalfPageUp",
	"HalfPageDown",
	"ToggleHelp",
	"ToggleKeyMenu",
	"ToggleRuler",
	"JumpLine",
	"ClearStatus",
	"ShellMode",
	"CommandMode",
	"AddTab",
	"PreviousTab",
	"NextTab",
	"NextSplit",
	"PreviousSplit",
	"Unsplit",
	"VSplit",
	"HSplit",
	"ToggleMacro",
	"PlayMacro",
	"Suspend",
	"ScrollUp",
	"ScrollDown",
	"SpawnMultiCursor",
	"SpawnMultiCursorSelect",
	"RemoveMultiCursor",
	"RemoveAllMultiCursors",
	"SkipMultiCursor",
}

// InfoOverrides is the list of actions which have been overriden
// by the infohandler
var InfoOverrides = map[string]InfoKeyAction{
	"CursorUp":      (*InfoHandler).CursorUp,
	"CursorDown":    (*InfoHandler).CursorDown,
	"InsertNewline": (*InfoHandler).InsertNewline,
	"InsertTab":     (*InfoHandler).InsertTab,
	"Escape":        (*InfoHandler).Escape,
	"Quit":          (*InfoHandler).Quit,
	"QuitAll":       (*InfoHandler).QuitAll,
}

func (h *InfoHandler) CursorUp() {
	h.UpHistory(h.History[h.PromptType])
}
func (h *InfoHandler) CursorDown() {
	h.DownHistory(h.History[h.PromptType])
}
func (h *InfoHandler) InsertTab() {
	// TODO: autocomplete
}
func (h *InfoHandler) InsertNewline() {
	h.DonePrompt(false)
}
func (h *InfoHandler) Quit() {
	h.DonePrompt(true)
}
func (h *InfoHandler) QuitAll() {
	h.DonePrompt(true)
}
func (h *InfoHandler) Escape() {
	h.DonePrompt(true)
}
