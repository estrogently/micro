package action

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	luar "layeh.com/gopher-luar"

	shellquote "github.com/kballard/go-shellquote"
	lua "github.com/yuin/gopher-lua"
	"github.com/zyedidia/micro/internal/buffer"
	"github.com/zyedidia/micro/internal/config"
	ulua "github.com/zyedidia/micro/internal/lua"
	"github.com/zyedidia/micro/internal/screen"
	"github.com/zyedidia/micro/internal/shell"
	"github.com/zyedidia/micro/internal/util"
)

// A Command contains information about how to execute a command
// It has the action for that command as well as a completer function
type Command struct {
	action    func(*BufPane, []string)
	completer buffer.Completer
}

var commands map[string]Command

func InitCommands() {
	commands = map[string]Command{
		"set":        {(*BufPane).SetCmd, OptionValueComplete},
		"reset":      {(*BufPane).ResetCmd, OptionValueComplete},
		"setlocal":   {(*BufPane).SetLocalCmd, OptionValueComplete},
		"show":       {(*BufPane).ShowCmd, OptionComplete},
		"showkey":    {(*BufPane).ShowKeyCmd, nil},
		"run":        {(*BufPane).RunCmd, nil},
		"bind":       {(*BufPane).BindCmd, nil},
		"unbind":     {(*BufPane).UnbindCmd, nil},
		"quit":       {(*BufPane).QuitCmd, nil},
		"goto":       {(*BufPane).GotoCmd, nil},
		"save":       {(*BufPane).SaveCmd, nil},
		"replace":    {(*BufPane).ReplaceCmd, nil},
		"replaceall": {(*BufPane).ReplaceAllCmd, nil},
		"vsplit":     {(*BufPane).VSplitCmd, buffer.FileComplete},
		"hsplit":     {(*BufPane).HSplitCmd, buffer.FileComplete},
		"tab":        {(*BufPane).NewTabCmd, buffer.FileComplete},
		"help":       {(*BufPane).HelpCmd, HelpComplete},
		"eval":       {(*BufPane).EvalCmd, nil},
		"log":        {(*BufPane).ToggleLogCmd, nil},
		"plugin":     {(*BufPane).PluginCmd, PluginComplete},
		"reload":     {(*BufPane).ReloadCmd, nil},
		"reopen":     {(*BufPane).ReopenCmd, nil},
		"cd":         {(*BufPane).CdCmd, buffer.FileComplete},
		"pwd":        {(*BufPane).PwdCmd, nil},
		"open":       {(*BufPane).OpenCmd, buffer.FileComplete},
		"tabswitch":  {(*BufPane).TabSwitchCmd, nil},
		"term":       {(*BufPane).TermCmd, nil},
		"memusage":   {(*BufPane).MemUsageCmd, nil},
		"retab":      {(*BufPane).RetabCmd, nil},
		"raw":        {(*BufPane).RawCmd, nil},
		"textfilter": {(*BufPane).TextFilterCmd, nil},
	}
}

// MakeCommand is a function to easily create new commands
// This can be called by plugins in Lua so that plugins can define their own commands
func LuaMakeCommand(name, function string, completer buffer.Completer) {
	action := LuaFunctionCommand(function)
	commands[name] = Command{action, completer}
}

// LuaFunctionCommand returns a normal function
// so that a command can be bound to a lua function
func LuaFunctionCommand(fn string) func(*BufPane, []string) {
	luaFn := strings.Split(fn, ".")
	if len(luaFn) <= 1 {
		return nil
	}
	plName, plFn := luaFn[0], luaFn[1]
	pl := config.FindPlugin(plName)
	if pl == nil {
		return nil
	}
	return func(bp *BufPane, args []string) {
		luaArgs := []lua.LValue{luar.New(ulua.L, bp), luar.New(ulua.L, args)}
		_, err := pl.Call(plFn, luaArgs...)
		if err != nil {
			screen.TermMessage(err)
		}
	}
}

// CommandEditAction returns a bindable function that opens a prompt with
// the given string and executes the command when the user presses
// enter
func CommandEditAction(prompt string) BufKeyAction {
	return func(h *BufPane) bool {
		InfoBar.Prompt("> ", prompt, "Command", nil, func(resp string, canceled bool) {
			if !canceled {
				MainTab().CurPane().HandleCommand(resp)
			}
		})
		return false
	}
}

// CommandAction returns a bindable function which executes the
// given command
func CommandAction(cmd string) BufKeyAction {
	return func(h *BufPane) bool {
		MainTab().CurPane().HandleCommand(cmd)
		return false
	}
}

var PluginCmds = []string{"list", "info", "version"}

// PluginCmd installs, removes, updates, lists, or searches for given plugins
func (h *BufPane) PluginCmd(args []string) {
	if len(args) <= 0 {
		InfoBar.Error("Not enough arguments, see 'help commands'")
		return
	}

	valid := true
	switch args[0] {
	case "list":
		for _, pl := range config.Plugins {
			var en string
			if pl.IsEnabled() {
				en = "enabled"
			} else {
				en = "disabled"
			}
			WriteLog(fmt.Sprintf("%s: %s", pl.Name, en))
			if pl.Default {
				WriteLog(" (default)\n")
			} else {
				WriteLog("\n")
			}
		}
		WriteLog("Default plugins come pre-installed with micro.")
	case "version":
		if len(args) <= 1 {
			InfoBar.Error("No plugin provided to give info for")
			return
		}
		found := false
		for _, pl := range config.Plugins {
			if pl.Name == args[1] {
				found = true
				if pl.Info == nil {
					InfoBar.Message("Sorry no version for", pl.Name)
					return
				}

				WriteLog("Version: " + pl.Info.Vstr + "\n")
			}
		}
		if !found {
			InfoBar.Message(args[1], "is not installed")
		}
	case "info":
		if len(args) <= 1 {
			InfoBar.Error("No plugin provided to give info for")
			return
		}
		found := false
		for _, pl := range config.Plugins {
			if pl.Name == args[1] {
				found = true
				if pl.Info == nil {
					InfoBar.Message("Sorry no info for ", pl.Name)
					return
				}

				var buffer bytes.Buffer
				buffer.WriteString("Name: ")
				buffer.WriteString(pl.Info.Name)
				buffer.WriteString("\n")
				buffer.WriteString("Description: ")
				buffer.WriteString(pl.Info.Desc)
				buffer.WriteString("\n")
				buffer.WriteString("Website: ")
				buffer.WriteString(pl.Info.Site)
				buffer.WriteString("\n")
				buffer.WriteString("Installation link: ")
				buffer.WriteString(pl.Info.Install)
				buffer.WriteString("\n")
				buffer.WriteString("Version: ")
				buffer.WriteString(pl.Info.Vstr)
				buffer.WriteString("\n")
				buffer.WriteString("Requirements:")
				buffer.WriteString("\n")
				for _, r := range pl.Info.Require {
					buffer.WriteString("    - ")
					buffer.WriteString(r)
					buffer.WriteString("\n")
				}

				WriteLog(buffer.String())
			}
		}
		if !found {
			InfoBar.Message(args[1], "is not installed")
			return
		}
	default:
		InfoBar.Error("Not a valid plugin command")
		return
	}

	if valid && h.Buf.Type != buffer.BTLog {
		OpenLogBuf(h)
	}
}

// RetabCmd changes all spaces to tabs or all tabs to spaces
// depending on the user's settings
func (h *BufPane) RetabCmd(args []string) {
	h.Buf.Retab()
}

// RawCmd opens a new raw view which displays the escape sequences micro
// is receiving in real-time
func (h *BufPane) RawCmd(args []string) {
	width, height := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	tp := NewTabFromPane(0, 0, width, height-iOffset, NewRawPane())
	Tabs.AddTab(tp)
	Tabs.SetActive(len(Tabs.List) - 1)
}

// TextFilterCmd filters the selection through the command.
// Selection goes to the command input.
// On successful run command output replaces the current selection.
func (h *BufPane) TextFilterCmd(args []string) {
	if len(args) == 0 {
		InfoBar.Error("usage: textfilter arguments")
		return
	}
	sel := h.Cursor.GetSelection()
	if len(sel) == 0 {
		h.Cursor.SelectWord()
		sel = h.Cursor.GetSelection()
	}
	var bout, berr bytes.Buffer
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(string(sel))
	cmd.Stderr = &berr
	cmd.Stdout = &bout
	err := cmd.Run()
	if err != nil {
		InfoBar.Error(err.Error() + " " + berr.String())
		return
	}
	h.Cursor.DeleteSelection()
	h.Buf.Insert(h.Cursor.Loc, bout.String())
}

// TabSwitchCmd switches to a given tab either by name or by number
func (h *BufPane) TabSwitchCmd(args []string) {
	if len(args) > 0 {
		num, err := strconv.Atoi(args[0])
		if err != nil {
			// Check for tab with this name

			found := false
			for i, t := range Tabs.List {
				if t.Panes[t.active].Name() == args[0] {
					Tabs.SetActive(i)
					found = true
				}
			}
			if !found {
				InfoBar.Error("Could not find tab: ", err)
			}
		} else {
			num--
			if num >= 0 && num < len(Tabs.List) {
				Tabs.SetActive(num)
			} else {
				InfoBar.Error("Invalid tab index")
			}
		}
	}
}

// CdCmd changes the current working directory
func (h *BufPane) CdCmd(args []string) {
	if len(args) > 0 {
		path, err := util.ReplaceHome(args[0])
		if err != nil {
			InfoBar.Error(err)
			return
		}
		err = os.Chdir(path)
		if err != nil {
			InfoBar.Error(err)
			return
		}
		wd, _ := os.Getwd()
		for _, b := range buffer.OpenBuffers {
			if len(b.Path) > 0 {
				b.Path, _ = util.MakeRelative(b.AbsPath, wd)
				if p, _ := filepath.Abs(b.Path); !strings.Contains(p, wd) {
					b.Path = b.AbsPath
				}
			}
		}
	}
}

// MemUsageCmd prints micro's memory usage
// Alloc shows how many bytes are currently in use
// Sys shows how many bytes have been requested from the operating system
// NumGC shows how many times the GC has been run
// Note that Go commonly reserves more memory from the OS than is currently in-use/required
// Additionally, even if Go returns memory to the OS, the OS does not always claim it because
// there may be plenty of memory to spare
func (h *BufPane) MemUsageCmd(args []string) {
	InfoBar.Message(util.GetMemStats())
}

// PwdCmd prints the current working directory
func (h *BufPane) PwdCmd(args []string) {
	wd, err := os.Getwd()
	if err != nil {
		InfoBar.Message(err.Error())
	} else {
		InfoBar.Message(wd)
	}
}

// OpenCmd opens a new buffer with a given filename
func (h *BufPane) OpenCmd(args []string) {
	if len(args) > 0 {
		filename := args[0]
		// the filename might or might not be quoted, so unquote first then join the strings.
		args, err := shellquote.Split(filename)
		if err != nil {
			InfoBar.Error("Error parsing args ", err)
			return
		}
		filename = strings.Join(args, " ")

		open := func() {
			b, err := buffer.NewBufferFromFile(filename, buffer.BTDefault)
			if err != nil {
				InfoBar.Error(err)
				return
			}
			h.OpenBuffer(b)
		}
		if h.Buf.Modified() {
			InfoBar.YNPrompt("Save changes to "+h.Buf.GetName()+" before closing? (y,n,esc)", func(yes, canceled bool) {
				if !canceled && !yes {
					open()
				} else if !canceled && yes {
					h.Save()
					open()
				}
			})
		} else {
			open()
		}
	} else {
		InfoBar.Error("No filename")
	}
}

// ToggleLogCmd toggles the log view
func (h *BufPane) ToggleLogCmd(args []string) {
	if h.Buf.Type != buffer.BTLog {
		OpenLogBuf(h)
	} else {
		h.Quit()
	}
}

// ReloadCmd reloads all files (syntax files, colorschemes...)
func (h *BufPane) ReloadCmd(args []string) {
	ReloadConfig()
}

func ReloadConfig() {
	config.InitRuntimeFiles()
	err := config.ReadSettings()
	if err != nil {
		screen.TermMessage(err)
	}
	config.InitGlobalSettings()
	InitBindings()
	InitCommands()

	err = config.InitColorscheme()
	if err != nil {
		screen.TermMessage(err)
	}

	for _, b := range buffer.OpenBuffers {
		b.UpdateRules()
	}
}

// ReopenCmd reopens the buffer (reload from disk)
func (h *BufPane) ReopenCmd(args []string) {
	if h.Buf.Modified() {
		InfoBar.YNPrompt("Save file before reopen?", func(yes, canceled bool) {
			if !canceled && yes {
				h.Save()
				h.Buf.ReOpen()
			} else if !canceled {
				h.Buf.ReOpen()
			}
		})
	} else {
		h.Buf.ReOpen()
	}
}

func (h *BufPane) openHelp(page string) error {
	if data, err := config.FindRuntimeFile(config.RTHelp, page).Data(); err != nil {
		return errors.New(fmt.Sprint("Unable to load help text", page, "\n", err))
	} else {
		helpBuffer := buffer.NewBufferFromString(string(data), page+".md", buffer.BTHelp)
		helpBuffer.SetName("Help " + page)

		if h.Buf.Type == buffer.BTHelp {
			h.OpenBuffer(helpBuffer)
		} else {
			h.HSplitBuf(helpBuffer)
		}
	}
	return nil
}

// HelpCmd tries to open the given help page in a horizontal split
func (h *BufPane) HelpCmd(args []string) {
	if len(args) < 1 {
		// Open the default help if the user just typed "> help"
		h.openHelp("help")
	} else {
		if config.FindRuntimeFile(config.RTHelp, args[0]) != nil {
			err := h.openHelp(args[0])
			if err != nil {
				InfoBar.Error(err)
			}
		} else {
			InfoBar.Error("Sorry, no help for ", args[0])
		}
	}
}

// VSplitCmd opens a vertical split with file given in the first argument
// If no file is given, it opens an empty buffer in a new split
func (h *BufPane) VSplitCmd(args []string) {
	if len(args) == 0 {
		// Open an empty vertical split
		h.VSplitAction()
		return
	}

	buf, err := buffer.NewBufferFromFile(args[0], buffer.BTDefault)
	if err != nil {
		InfoBar.Error(err)
		return
	}

	h.VSplitBuf(buf)
}

// HSplitCmd opens a horizontal split with file given in the first argument
// If no file is given, it opens an empty buffer in a new split
func (h *BufPane) HSplitCmd(args []string) {
	if len(args) == 0 {
		// Open an empty horizontal split
		h.HSplitAction()
		return
	}

	buf, err := buffer.NewBufferFromFile(args[0], buffer.BTDefault)
	if err != nil {
		InfoBar.Error(err)
		return
	}

	h.HSplitBuf(buf)
}

// EvalCmd evaluates a lua expression
func (h *BufPane) EvalCmd(args []string) {
	InfoBar.Error("Eval unsupported")
}

// NewTabCmd opens the given file in a new tab
func (h *BufPane) NewTabCmd(args []string) {
	width, height := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	if len(args) > 0 {
		for _, a := range args {
			b, err := buffer.NewBufferFromFile(a, buffer.BTDefault)
			if err != nil {
				InfoBar.Error(err)
				return
			}
			tp := NewTabFromBuffer(0, 0, width, height-1-iOffset, b)
			Tabs.AddTab(tp)
			Tabs.SetActive(len(Tabs.List) - 1)
		}
	} else {
		b := buffer.NewBufferFromString("", "", buffer.BTDefault)
		tp := NewTabFromBuffer(0, 0, width, height-iOffset, b)
		Tabs.AddTab(tp)
		Tabs.SetActive(len(Tabs.List) - 1)
	}
}

func SetGlobalOptionNative(option string, nativeValue interface{}) error {
	config.GlobalSettings[option] = nativeValue

	if option == "colorscheme" {
		// LoadSyntaxFiles()
		config.InitColorscheme()
		for _, b := range buffer.OpenBuffers {
			b.UpdateRules()
		}
	} else if option == "infobar" || option == "keymenu" {
		Tabs.Resize()
	} else if option == "mouse" {
		if !nativeValue.(bool) {
			screen.Screen.DisableMouse()
		} else {
			screen.Screen.EnableMouse()
		}
		// autosave option has been removed
		// } else if option == "autosave" {
		// 	if nativeValue.(float64) > 0 {
		// 		config.SetAutoTime(int(nativeValue.(float64)))
		// 		config.StartAutoSave()
		// 	} else {
		// 		config.SetAutoTime(0)
		// 	}
	} else if option == "paste" {
		screen.Screen.SetPaste(nativeValue.(bool))
	} else {
		for _, pl := range config.Plugins {
			if option == pl.Name {
				if nativeValue.(bool) && !pl.Loaded {
					pl.Load()
					_, err := pl.Call("init")
					if err != nil && err != config.ErrNoSuchFunction {
						screen.TermMessage(err)
					}
				} else if !nativeValue.(bool) && pl.Loaded {
					_, err := pl.Call("deinit")
					if err != nil && err != config.ErrNoSuchFunction {
						screen.TermMessage(err)
					}
				}
			}
		}
	}

	for _, b := range buffer.OpenBuffers {
		b.SetOptionNative(option, nativeValue)
	}

	return config.WriteSettings(config.ConfigDir + "/settings.json")
}

func SetGlobalOption(option, value string) error {
	if _, ok := config.GlobalSettings[option]; !ok {
		return config.ErrInvalidOption
	}

	nativeValue, err := config.GetNativeValue(option, config.GlobalSettings[option], value)
	if err != nil {
		return err
	}

	return SetGlobalOptionNative(option, nativeValue)
}

// ResetCmd resets a setting to its default value
func (h *BufPane) ResetCmd(args []string) {
	if len(args) < 1 {
		InfoBar.Error("Not enough arguments")
		return
	}

	option := args[0]

	defaultGlobals := config.DefaultGlobalSettings()
	defaultLocals := config.DefaultCommonSettings()

	if _, ok := defaultGlobals[option]; ok {
		SetGlobalOptionNative(option, defaultGlobals[option])
		return
	}
	if _, ok := defaultLocals[option]; ok {
		h.Buf.SetOptionNative(option, defaultLocals[option])
		return
	}
	InfoBar.Error(config.ErrInvalidOption)
}

// SetCmd sets an option
func (h *BufPane) SetCmd(args []string) {
	if len(args) < 2 {
		InfoBar.Error("Not enough arguments")
		return
	}

	option := args[0]
	value := args[1]

	err := SetGlobalOption(option, value)
	if err == config.ErrInvalidOption {
		err := h.Buf.SetOption(option, value)
		if err != nil {
			InfoBar.Error(err)
		}
	} else if err != nil {
		InfoBar.Error(err)
	}
}

// SetLocalCmd sets an option local to the buffer
func (h *BufPane) SetLocalCmd(args []string) {
	if len(args) < 2 {
		InfoBar.Error("Not enough arguments")
		return
	}

	option := args[0]
	value := args[1]

	err := h.Buf.SetOption(option, value)
	if err != nil {
		InfoBar.Error(err)
	}
}

// ShowCmd shows the value of the given option
func (h *BufPane) ShowCmd(args []string) {
	if len(args) < 1 {
		InfoBar.Error("Please provide an option to show")
		return
	}

	var option interface{}
	if opt, ok := h.Buf.Settings[args[0]]; ok {
		option = opt
	} else if opt, ok := config.GlobalSettings[args[0]]; ok {
		option = opt
	}

	if option == nil {
		InfoBar.Error(args[0], " is not a valid option")
		return
	}

	InfoBar.Message(option)
}

// ShowKeyCmd displays the action that a key is bound to
func (h *BufPane) ShowKeyCmd(args []string) {
	if len(args) < 1 {
		InfoBar.Error("Please provide a key to show")
		return
	}

	if action, ok := config.Bindings[args[0]]; ok {
		InfoBar.Message(action)
	} else {
		InfoBar.Message(args[0], " has no binding")
	}
}

// BindCmd creates a new keybinding
func (h *BufPane) BindCmd(args []string) {
	if len(args) < 2 {
		InfoBar.Error("Not enough arguments")
		return
	}

	_, err := TryBindKey(args[0], args[1], true)
	if err != nil {
		InfoBar.Error(err)
	}
}

// UnbindCmd binds a key to its default action
func (h *BufPane) UnbindCmd(args []string) {
	if len(args) < 1 {
		InfoBar.Error("Not enough arguments")
		return
	}

	err := UnbindKey(args[0])
	if err != nil {
		InfoBar.Error(err)
	}
}

// RunCmd runs a shell command in the background
func (h *BufPane) RunCmd(args []string) {
	runf, err := shell.RunBackgroundShell(shellquote.Join(args...))
	if err != nil {
		InfoBar.Error(err)
	} else {
		go func() {
			InfoBar.Message(runf())
			screen.Redraw()
		}()
	}
}

// QuitCmd closes the main view
func (h *BufPane) QuitCmd(args []string) {
	h.Quit()
}

// GotoCmd is a command that will send the cursor to a certain
// position in the buffer
// For example: `goto line`, or `goto line:col`
func (h *BufPane) GotoCmd(args []string) {
	if len(args) <= 0 {
		InfoBar.Error("Not enough arguments")
	} else {
		h.RemoveAllMultiCursors()
		if strings.Contains(args[0], ":") {
			parts := strings.SplitN(args[0], ":", 2)
			line, err := strconv.Atoi(parts[0])
			if err != nil {
				InfoBar.Error(err)
				return
			}
			col, err := strconv.Atoi(parts[1])
			if err != nil {
				InfoBar.Error(err)
				return
			}
			line = util.Clamp(line-1, 0, h.Buf.LinesNum()-1)
			col = util.Clamp(col-1, 0, utf8.RuneCount(h.Buf.LineBytes(line)))
			h.Cursor.GotoLoc(buffer.Loc{col, line})
		} else {
			line, err := strconv.Atoi(args[0])
			if err != nil {
				InfoBar.Error(err)
				return
			}
			line = util.Clamp(line-1, 0, h.Buf.LinesNum()-1)
			h.Cursor.GotoLoc(buffer.Loc{0, line})
		}
		h.Relocate()
	}
}

// SaveCmd saves the buffer optionally with an argument file name
func (h *BufPane) SaveCmd(args []string) {
	if len(args) == 0 {
		h.Save()
	} else {
		h.Buf.SaveAs(args[0])
	}
}

// ReplaceCmd runs search and replace
func (h *BufPane) ReplaceCmd(args []string) {
	if len(args) < 2 || len(args) > 4 {
		// We need to find both a search and replace expression
		InfoBar.Error("Invalid replace statement: " + strings.Join(args, " "))
		return
	}

	all := false
	noRegex := false

	foundSearch := false
	foundReplace := false
	var search string
	var replaceStr string
	for _, arg := range args {
		switch arg {
		case "-a":
			all = true
		case "-l":
			noRegex = true
		default:
			if !foundSearch {
				foundSearch = true
				search = arg
			} else if !foundReplace {
				foundReplace = true
				replaceStr = arg
			} else {
				InfoBar.Error("Invalid flag: " + arg)
				return
			}
		}
	}

	if noRegex {
		search = regexp.QuoteMeta(search)
	}

	replace := []byte(replaceStr)

	var regex *regexp.Regexp
	var err error
	if h.Buf.Settings["ignorecase"].(bool) {
		regex, err = regexp.Compile("(?im)" + search)
	} else {
		regex, err = regexp.Compile("(?m)" + search)
	}
	if err != nil {
		// There was an error with the user's regex
		InfoBar.Error(err)
		return
	}

	nreplaced := 0
	start := h.Buf.Start()
	// end := h.Buf.End()
	// if h.Cursor.HasSelection() {
	// 	start = h.Cursor.CurSelection[0]
	// 	end = h.Cursor.CurSelection[1]
	// }
	if all {
		nreplaced = h.Buf.ReplaceRegex(start, h.Buf.End(), regex, replace)
	} else {
		inRange := func(l buffer.Loc) bool {
			return l.GreaterEqual(start) && l.LessEqual(h.Buf.End())
		}

		searchLoc := start
		searching := true
		var doReplacement func()
		doReplacement = func() {
			locs, found, err := h.Buf.FindNext(search, start, h.Buf.End(), searchLoc, true, !noRegex)
			if err != nil {
				InfoBar.Error(err)
				return
			}
			if !found || !inRange(locs[0]) || !inRange(locs[1]) {
				h.Cursor.ResetSelection()
				h.Buf.RelocateCursors()
				return
			}

			h.Cursor.SetSelectionStart(locs[0])
			h.Cursor.SetSelectionEnd(locs[1])

			InfoBar.YNPrompt("Perform replacement (y,n,esc)", func(yes, canceled bool) {
				if !canceled && yes {
					h.Buf.Replace(locs[0], locs[1], replaceStr)

					searchLoc = locs[0]
					searchLoc.X += utf8.RuneCount(replace)
					h.Cursor.Loc = searchLoc
					nreplaced++
				} else if !canceled && !yes {
					searchLoc = locs[0]
					searchLoc.X += utf8.RuneCount(replace)
				} else if canceled {
					h.Cursor.ResetSelection()
					h.Buf.RelocateCursors()
					return
				}
				if searching {
					doReplacement()
				}
			})
		}
		doReplacement()
	}

	h.Buf.RelocateCursors()

	if nreplaced > 1 {
		InfoBar.Message("Replaced ", nreplaced, " occurrences of ", search)
	} else if nreplaced == 1 {
		InfoBar.Message("Replaced ", nreplaced, " occurrence of ", search)
	} else {
		InfoBar.Message("Nothing matched ", search)
	}
}

// ReplaceAllCmd replaces search term all at once
func (h *BufPane) ReplaceAllCmd(args []string) {
	// aliased to Replace command
	h.ReplaceCmd(append(args, "-a"))
}

// TermCmd opens a terminal in the current view
func (h *BufPane) TermCmd(args []string) {
	ps := MainTab().Panes

	if len(args) == 0 {
		sh := os.Getenv("SHELL")
		if sh == "" {
			InfoBar.Error("Shell environment not found")
			return
		}
		args = []string{sh}
	}

	term := func(i int, newtab bool) {
		t := new(shell.Terminal)
		t.Start(args, false, true, "", nil)

		id := h.ID()
		if newtab {
			h.AddTab()
			i = 0
			id = MainTab().Panes[0].ID()
		} else {
			MainTab().Panes[i].Close()
		}

		v := h.GetView()
		MainTab().Panes[i] = NewTermPane(v.X, v.Y, v.Width, v.Height, t, id)
		MainTab().SetActive(i)
	}

	// If there is only one open file we make a new tab instead of overwriting it
	newtab := len(MainTab().Panes) == 1 && len(Tabs.List) == 1

	if newtab {
		term(0, true)
		return
	}

	for i, p := range ps {
		if p.ID() == h.ID() {
			if h.Buf.Modified() {
				InfoBar.YNPrompt("Save changes to "+h.Buf.GetName()+" before closing? (y,n,esc)", func(yes, canceled bool) {
					if !canceled && !yes {
						term(i, false)
					} else if !canceled && yes {
						h.Save()
						term(i, false)
					}
				})
			} else {
				term(i, false)
			}
		}
	}
}

// HandleCommand handles input from the user
func (h *BufPane) HandleCommand(input string) {
	args, err := shellquote.Split(input)
	if err != nil {
		InfoBar.Error("Error parsing args ", err)
		return
	}

	inputCmd := args[0]

	if _, ok := commands[inputCmd]; !ok {
		InfoBar.Error("Unknown command ", inputCmd)
	} else {
		WriteLog("> " + input + "\n")
		commands[inputCmd].action(h, args[1:])
		WriteLog("\n")
	}
}
