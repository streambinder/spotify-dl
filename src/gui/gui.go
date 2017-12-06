package gui

import (
	"fmt"
	"log"
	"strings"
	"sync"

	spttb_logger "logger"

	"github.com/fatih/color"
	"github.com/jroimartin/gocui"
)

const (
	OptionNil = -1
	_
	// PromptNotDismissable = iota
	PromptDismissable = iota
	PromptDismissableWithExit
	_

	PanelLeftTop = iota
	PanelLeftBottom
	PanelRight
	_
	OrientationLeft = iota
	OrientationCenter
	OrientationRight
	_
	FontColorBlack = iota
	FontColorRed
	FontColorGreen
	FontColorYellow
	FontColorBlue
	FontColorMagenta
	FontColorCyan
	FontColorWhite
	_
	FontStyleBold = iota
	FontStyleUnderline
	FontStyleReverse
	_
	LogWrite = iota
	LogNoWrite
)

var (
	Panels = map[int]string{
		PanelLeftTop:    "GuiPanelLeftTop",
		PanelLeftBottom: "GuiPanelLeftBottom",
		PanelRight:      "GuiPanelRight",
	}
	FontColors = map[int]color.Attribute{
		FontColorBlack:   color.FgBlack,
		FontColorRed:     color.FgRed,
		FontColorGreen:   color.FgGreen,
		FontColorYellow:  color.FgYellow,
		FontColorBlue:    color.FgBlue,
		FontColorMagenta: color.FgMagenta,
		FontColorCyan:    color.FgCyan,
		FontColorWhite:   color.FgWhite,
	}
	FontStyles = map[int]color.Attribute{
		FontStyleBold: color.Bold,
	}

	gui_ready          chan *gocui.Gui
	gui_prompt_dismiss chan bool
	gui_append_mutex   sync.Mutex
	gui_prompt_mutex   sync.Mutex

	singleton *Gui
)

type Gui struct {
	*gocui.Gui
	Width   int
	Height  int
	Verbose bool
	Closing chan bool
	Logger  *spttb_logger.Logger
}

func Build(verbose bool) *Gui {
	var gui *gocui.Gui
	gui_ready = make(chan *gocui.Gui)
	go Run()
	gui = <-gui_ready
	gui_width, gui_height := gui.Size()

	singleton = &Gui{
		gui,
		gui_width,
		gui_height,
		verbose,
		make(chan bool),
		nil,
	}
	return singleton
}

func (gui *Gui) LinkLogger(logger *spttb_logger.Logger) error {
	gui.Logger = logger
	return nil
}

func Run() {
	gui, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	defer gui.Close()

	gui.SetManagerFunc(GuiSTDLayout)

	if err := gui.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, GuiClose); err != nil {
		log.Panicln(err)
	}

	gui_ready <- gui

	if err := gui.MainLoop(); err != nil {
		if err != gocui.ErrQuit {
			log.Panicln(err)
		}
	}
}

func (gui *Gui) Append(message string, panel int, options ...int) error {
	gui_append_mutex.Lock()
	defer gui_append_mutex.Unlock()
	view, err := gui.View(Panels[panel])
	if err != nil {
		return err
	} else {
		if (len(options) <= 3 || options[3] == LogWrite) && gui.Logger != nil {
			gui.Logger.Append(message)
		}
		gui.Update(func(gui *gocui.Gui) error {
			if len(options) > 2 && options[2] >= 0 {
				message = MessageStyle(message, options[2])
			}
			if len(options) > 0 && options[0] >= 0 {
				message = MessageOrientate(message, view, options[0])
			} else {
				message = MessageOrientate(message, view, OrientationLeft)
			}
			if len(options) > 1 && options[1] >= 0 {
				message = MessageColor(message, options[1])
			}
			fmt.Fprintln(view, message)
			return nil
		})
	}
	return nil
}

func (gui *Gui) ClearAppend(message string, panel int, options ...int) error {
	view, err := gui.View(Panels[panel])
	if err != nil {
		return err
	} else {
		view.Clear()
		return gui.Append(message, panel, options...)
	}
	return nil
}

func (gui *Gui) ErrAppend(message string, panel int, options ...int) error {
	return gui.Append(message, panel, ReplaceOptions(options, 1, FontColorRed)...)
}

func (gui *Gui) WarnAppend(message string, panel int, options ...int) error {
	return gui.Append(message, panel, ReplaceOptions(options, 1, FontColorYellow)...)
}

func (gui *Gui) DebugAppend(message string, panel int, options ...int) error {
	if !gui.Verbose {
		if gui.Logger != nil {
			return gui.Logger.Append(message)
		}
		return nil
	} else {
		return gui.Append(message, panel, ReplaceOptions(options, 1, FontColorMagenta)...)
	}
}

func (gui *Gui) Prompt(message string, options ...int) error {
	gui_prompt_mutex.Lock()
	defer gui_prompt_mutex.Unlock()
	gui_prompt_dismiss = make(chan bool)
	if (len(options) <= 1 || options[1] == LogWrite) && gui.Logger != nil {
		gui.Logger.Append(message)
	}
	gui.Update(func(gui *gocui.Gui) error {
		var (
			view *gocui.View
			err  error
		)
		gui_weight, gui_height := gui.Size()
		if view, err = gui.SetView("GuiPrompt",
			gui_weight/2-(len(message)/2)-2, gui_height/2,
			gui_weight/2+(len(message)/2), gui_height/2+2); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			fmt.Fprintln(view, message)
			if len(options) == 0 {
				options = append(options, PromptDismissable)
			}
			if options[0] == PromptDismissableWithExit {
				gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPromptAndClose)
			} else {
				gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPrompt)
			}
		}
		return nil
	})
	<-gui_prompt_dismiss
	return nil
}

func (gui *Gui) PromptInput(message string, options ...int) bool {
	gui_prompt_mutex.Lock()
	defer gui_prompt_mutex.Unlock()
	gui_prompt_dismiss = make(chan bool)
	gui.Update(func(gui *gocui.Gui) error {
		var (
			view *gocui.View
			err  error
		)
		gui_weight, gui_height := gui.Size()
		if view, err = gui.SetView("GuiPrompt",
			gui_weight/2-(len(message)/2)-2, gui_height/2,
			gui_weight/2+(len(message)/2), gui_height/2+2); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			fmt.Fprintln(view, MessageOrientate(fmt.Sprintf("%s\n(ESC to cancel, ENTER to ok)", message), view, OrientationCenter))
			if len(options) == 0 {
				options = append(options, PromptDismissable)
			}
			gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPromptWithInputOk)
			gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPromptWithInputNok)
		}
		return nil
	})
	return <-gui_prompt_dismiss
}

/*
 * Auxiliary functions
 */
func GuiSTDLayout(gui *gocui.Gui) error {
	gui_max_weight, gui_max_height := gui.Size()
	if view, err := gui.SetView("GuiPanelLeftTop", 0, 0,
		gui_max_weight/3, gui_max_height/2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		view.Autoscroll = true
	}
	if view, err := gui.SetView("GuiPanelLeftBottom", 0, gui_max_height/2+1,
		gui_max_weight/3, gui_max_height-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		view.Autoscroll = true
	}
	if view, err := gui.SetView("GuiPanelRight", gui_max_weight/3+1, 0,
		gui_max_weight-1, gui_max_height-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		view.Autoscroll = true
	}

	return nil
}

func MessageOrientate(message string, view *gocui.View, orientation int) string {
	var message_lines []string
	var line_length, _ = view.Size()
	for _, line := range strings.Split(message, "\n") {
		if len(line) < line_length {
			line_spacing := (line_length - len(line)) / 2
			if orientation == OrientationLeft {
				line = line
			} else if orientation == OrientationCenter {
				line = strings.Repeat(" ", line_spacing) +
					line + strings.Repeat(" ", line_spacing)
			} else if orientation == OrientationRight {
				line = strings.Repeat(" ", line_spacing*2-1) + line
			}
		}
		message_lines = append(message_lines, line)
	}
	return strings.Join(message_lines, "\n")
}

func MessageColor(message string, color_const int) string {
	color_func := color.New(FontColors[color_const])
	return color_func.Sprintf(message)
}

func MessageStyle(message string, style_const int) string {
	style_func := color.New(FontStyles[style_const])
	return style_func.Sprintf(message)
}

func ReplaceOptions(options []int, element_index int, element_value int) []int {
	if len(options) > element_index {
		options[element_index] = element_value
	} else if len(options) == element_index {
		options = append(options, element_value)
	} else {
		for i := 0; i < element_index; i++ {
			options = append(options, -1)
		}
		options = append(options, element_value)
	}
	return options
}

func GuiDismissPrompt(gui *gocui.Gui, view *gocui.View) error {
	gui.Update(func(gui *gocui.Gui) error {
		gui.DeleteView("GuiPrompt")
		return nil
	})
	gui.DeleteKeybinding("", gocui.KeyEnter, gocui.ModNone)
	gui_prompt_dismiss <- true
	return nil
}

func GuiDismissPromptAndClose(gui *gocui.Gui, view *gocui.View) error {
	GuiDismissPrompt(gui, view)
	return GuiClose(gui, view)
}

func GuiDismissPromptWithInput(gui *gocui.Gui, view *gocui.View) error {
	gui.Update(func(gui *gocui.Gui) error {
		gui.DeleteView("GuiPrompt")
		return nil
	})
	gui.DeleteKeybinding("", gocui.KeyEnter, gocui.ModNone)
	gui.DeleteKeybinding("", gocui.KeyEsc, gocui.ModNone)
	return nil
}

func GuiDismissPromptWithInputOk(gui *gocui.Gui, view *gocui.View) error {
	GuiDismissPromptWithInput(gui, view)
	gui_prompt_dismiss <- true
	return nil
}

func GuiDismissPromptWithInputNok(gui *gocui.Gui, view *gocui.View) error {
	GuiDismissPromptWithInput(gui, view)
	gui_prompt_dismiss <- false
	return nil
}

func GuiClose(gui *gocui.Gui, view *gocui.View) error {
	singleton.Closing <- true
	gui.DeleteKeybinding("", gocui.KeyCtrlC, gocui.ModNone)
	return gocui.ErrQuit
}
