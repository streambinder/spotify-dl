package gui

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"

	spttb_logger "logger"

	"github.com/fatih/color"
	"github.com/jroimartin/gocui"
)

const (
	OptionNil = 1 << iota
	_
	// PromptNotDismissable =  1 << iota
	PromptDismissable
	PromptDismissableWithExit
	_

	PanelLeftTop
	PanelLeftBottom
	PanelRight
	PanelLoading
	_
	OrientationLeft
	OrientationCenter
	OrientationRight
	_
	FontColorBlack
	FontColorRed
	FontColorGreen
	FontColorYellow
	FontColorBlue
	FontColorMagenta
	FontColorCyan
	FontColorWhite
	_
	FontStyleBold
	FontStyleUnderline
	FontStyleReverse
	_
	ParagraphStyleStandard
	ParagraphStyleAutoReturn
	_
	LogWrite
	LogNoWrite
)

var (
	Panels = map[int]string{
		PanelLeftTop:    "GuiPanelLeftTop",
		PanelLeftBottom: "GuiPanelLeftBottom",
		PanelRight:      "GuiPanelRight",
		PanelLoading:    "GuiPanelLoading",
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
	gui_prompt_input   chan string
	gui_prompt_mutex   sync.Mutex
	gui_append_mutex   sync.Mutex
	gui_loading_max    int = 100
	gui_loading_ctr    int
	gui_loading_sprint = color.New(color.BgWhite).SprintFunc()(" ")

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

func (gui *Gui) Append(message string, options uint64) error {
	gui_append_mutex.Lock()
	defer gui_append_mutex.Unlock()
	var (
		view  *gocui.View
		err   error
		panel uint64
	)
	if (options & PanelRight) != 0 {
		panel = PanelRight
	} else if (options & PanelLeftTop) != 0 {
		panel = PanelLeftTop
	} else if (options & PanelLeftBottom) != 0 {
		panel = PanelLeftBottom
	}
	view, err = gui.View(Panels[int(panel)])
	if err != nil {
		return err
	} else {
		gui.Update(func(gui *gocui.Gui) error {
			if (options&ParagraphStyleStandard) != 0 ||
				(options&ParagraphStyleAutoReturn) != 0 {
				width, _ := view.Size()
				if (options & ParagraphStyleStandard) != 0 {
					message = MessageParagraphStyle(message, ParagraphStyleStandard, width)
				} else if (options & ParagraphStyleAutoReturn) != 0 {
					message = MessageParagraphStyle(message, ParagraphStyleAutoReturn, width)
				}
			}
			if (options&FontStyleBold) != 0 ||
				(options&FontStyleUnderline) != 0 ||
				(options&FontStyleReverse) != 0 {
				if (options & FontStyleBold) != 0 {
					message = MessageStyle(message, FontStyleBold)
				} else if (options & FontStyleUnderline) != 0 {
					message = MessageStyle(message, FontStyleUnderline)
				} else if (options & FontStyleReverse) != 0 {
					message = MessageStyle(message, FontStyleReverse)
				}
			}
			if (options&OrientationLeft) != 0 ||
				(options&OrientationCenter) != 0 ||
				(options&OrientationRight) != 0 {
				if (options & OrientationLeft) != 0 {
					message = MessageOrientate(message, view, OrientationLeft)
				} else if (options & OrientationCenter) != 0 {
					message = MessageOrientate(message, view, OrientationCenter)
				} else if (options & OrientationRight) != 0 {
					message = MessageOrientate(message, view, OrientationRight)
				}
			}
			if (options&FontColorBlack) != 0 ||
				(options&FontColorRed) != 0 ||
				(options&FontColorGreen) != 0 ||
				(options&FontColorYellow) != 0 ||
				(options&FontColorBlue) != 0 ||
				(options&FontColorMagenta) != 0 ||
				(options&FontColorCyan) != 0 ||
				(options&FontColorWhite) != 0 {
				if (options & FontColorBlack) != 0 {
					message = MessageColor(message, FontColorBlack)
				} else if (options & FontColorRed) != 0 {
					message = MessageColor(message, FontColorRed)
				} else if (options & FontColorGreen) != 0 {
					message = MessageColor(message, FontColorGreen)
				} else if (options & FontColorYellow) != 0 {
					message = MessageColor(message, FontColorYellow)
				} else if (options & FontColorBlue) != 0 {
					message = MessageColor(message, FontColorBlue)
				} else if (options & FontColorMagenta) != 0 {
					message = MessageColor(message, FontColorMagenta)
				} else if (options & FontColorCyan) != 0 {
					message = MessageColor(message, FontColorCyan)
				} else if (options & FontColorWhite) != 0 {
					message = MessageColor(message, FontColorWhite)
				}
			}
			fmt.Fprintln(view, " "+message)
			return nil
		})
	}
	return nil
}

func (gui *Gui) ClearAppend(message string, options uint64) error {
	var (
		view  *gocui.View
		err   error
		panel uint64
	)
	if (options & PanelRight) != 0 {
		panel = PanelRight
	} else if (options & PanelLeftTop) != 0 {
		panel = PanelLeftTop
	} else if (options & PanelLeftBottom) != 0 {
		panel = PanelLeftBottom
	}
	view, err = gui.View(Panels[int(panel)])
	if err != nil {
		return err
	} else {
		view.Clear()
		return gui.Append(message, options|panel)
	}
	return nil
}

func (gui *Gui) ErrAppend(message string, options uint64) error {
	if (options&LogWrite) != 0 && gui.Logger != nil {
		gui.Logger.Append(fmt.Sprintf("[ERROR] %s", message))
	}
	return gui.Append(message, options|FontColorRed|ParagraphStyleAutoReturn)
}

func (gui *Gui) WarnAppend(message string, options uint64) error {
	if (options&LogWrite) != 0 && gui.Logger != nil {
		gui.Logger.Append(fmt.Sprintf("[WARNING] %s", message))
	}
	return gui.Append(message, options|FontColorYellow|ParagraphStyleAutoReturn)
}

func (gui *Gui) DebugAppend(message string, options uint64) error {
	if (options&LogWrite) != 0 && gui.Logger != nil {
		gui.Logger.Append(fmt.Sprintf("[DEBUG] %s", message))
	}
	if !gui.Verbose {
		return nil
	} else {
		return gui.Append(message, options|ParagraphStyleAutoReturn|FontColorMagenta)
	}
}

func (gui *Gui) Prompt(message string, options uint64) error {
	gui_prompt_mutex.Lock()
	defer gui_prompt_mutex.Unlock()
	gui_prompt_dismiss = make(chan bool)
	if (options&LogWrite) != 0 && gui.Logger != nil {
		gui.Logger.Append(message)
	}
	gui.Update(func(gui *gocui.Gui) error {
		var (
			view *gocui.View
			err  error
		)
		gui_width, gui_height := gui.Size()
		if view, err = gui.SetView("GuiPrompt",
			gui_width/2-(len(message)/2)-2, gui_height/2,
			gui_width/2+(len(message)/2), gui_height/2+2); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			fmt.Fprintln(view, message)
			if (options & PromptDismissableWithExit) != 0 {
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

func (gui *Gui) PromptInput(message string, options uint64) bool {
	gui_prompt_mutex.Lock()
	defer gui_prompt_mutex.Unlock()
	gui_prompt_dismiss = make(chan bool)
	gui.Update(func(gui *gocui.Gui) error {
		var (
			view *gocui.View
			err  error
		)
		message = fmt.Sprintf("%s\n\nPress TAB to cancel, ENTER to confirm.", message)
		gui_width, gui_height := gui.Size()
		needed_width, needed_height := 0, strings.Count(message, "\n")
		for _, line := range strings.Split(message, "\n") {
			if len(line) > needed_width {
				needed_width = len(line)
			}
		}
		needed_width += 2
		if view, err = gui.SetView("GuiPrompt",
			gui_width/2-(int(needed_width/2))-2, (gui_height/2)-int(math.Floor(float64(needed_height/2))),
			gui_width/2+(int(needed_width/2)), (gui_height/2)+int(math.Ceil(float64(needed_height/2)))+3); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPromptWithInputOk)
			gui.SetKeybinding("", gocui.KeyTab, gocui.ModNone, GuiDismissPromptWithInputNok)
			fmt.Fprintln(view, MessageOrientate(message, view, OrientationCenter))
		}
		return nil
	})
	return <-gui_prompt_dismiss
}

func (gui *Gui) PromptInputMessage(message string, options uint64) string {
	gui_prompt_mutex.Lock()
	defer gui_prompt_mutex.Unlock()
	gui_prompt_input = make(chan string)
	gui.Update(func(gui *gocui.Gui) error {
		var (
			view *gocui.View
			err  error
		)
		gui_width, gui_height := gui.Size()
		if view, err = gui.SetView("GuiPrompt",
			gui_width/2-50, (gui_height/2)-1,
			gui_width/2+50, (gui_height/2)+1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			view.Editable = true
			view.Title = fmt.Sprintf(" %s ", message)
			_ = view.SetCursor(0, 0)
			gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPromptWithInputMessage)
			_, _ = gui.SetCurrentView("GuiPrompt")
		}
		return nil
	})
	return strings.Replace(<-gui_prompt_input, "\n", "", -1)
}

/*
 * Auxiliary functions
 */
func GuiSTDLayout(gui *gocui.Gui) error {
	gui_max_width, gui_max_height := gui.Size()
	if view, err := gui.SetView("GuiPanelLeftTop", 0, 0,
		gui_max_width/3, gui_max_height/2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		view.Autoscroll = true
		view.Title = strings.ToUpper(" SpotiTube ")
		fmt.Fprintln(view, "\n")
	}
	if view, err := gui.SetView("GuiPanelLeftBottom", 0, gui_max_height/2+1,
		gui_max_width/3, gui_max_height-4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		view.Autoscroll = true
		view.Title = strings.ToUpper(" Informations ")
		fmt.Fprintln(view, "\n")
	}
	if view, err := gui.SetView("GuiPanelRight", gui_max_width/3+1, 0,
		gui_max_width-1, gui_max_height-4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		view.Autoscroll = true
		view.Title = strings.ToUpper(" Status ")
		fmt.Fprintln(view, "\n")
	}
	if _, err := gui.SetView("GuiPanelLoading", 0, gui_max_height-3,
		gui_max_width-1, gui_max_height-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	return nil
}

func (gui *Gui) LoadingSetMax(max int) error {
	gui_loading_max = max
	return nil
}

func (gui *Gui) LoadingFill() error {
	gui.Update(func(gui *gocui.Gui) error {
		view, err := gui.View(Panels[PanelLoading])
		if err != nil {
			return err
		} else {
			max_width, _ := view.Size()
			view.Clear()
			fmt.Fprint(view, strings.Repeat(gui_loading_sprint, max_width))
		}
		return nil
	})
	return nil
}

func (gui *Gui) LoadingIncrease() error {
	gui.Update(func(gui *gocui.Gui) error {
		view, err := gui.View(Panels[PanelLoading])
		if err != nil {
			return err
		} else {
			max_width, _ := view.Size()
			view.Clear()
			fmt.Fprint(view, strings.Repeat(gui_loading_sprint, gui_loading_ctr*max_width/gui_loading_max))
			gui_loading_ctr += 1
		}
		return nil
	})
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

func MessageParagraphStyle(message string, style_const int, width int) string {
	if style_const == ParagraphStyleAutoReturn {
		var message_paragraph string
		for len(message) > 0 {
			if len(message) < width {
				message_paragraph = message_paragraph + message[:len(message)]
				message = ""
			} else {
				message_paragraph = message_paragraph + message[:width] + "\n"
				message = message[width:]
			}
		}
		return message_paragraph
	}
	return message
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
	gui.DeleteKeybinding("", gocui.KeyTab, gocui.ModNone)
	return nil
}

func GuiDismissPromptWithInputOk(gui *gocui.Gui, view *gocui.View) error {
	if err := GuiDismissPromptWithInput(gui, view); err != nil {
		return err
	}
	gui_prompt_dismiss <- true
	return nil
}

func GuiDismissPromptWithInputNok(gui *gocui.Gui, view *gocui.View) error {
	if err := GuiDismissPromptWithInput(gui, view); err != nil {
		return err
	}
	gui_prompt_dismiss <- false
	return nil
}

func GuiDismissPromptWithInputMessage(gui *gocui.Gui, view *gocui.View) error {
	gui.Update(func(gui *gocui.Gui) error {
		gui.DeleteView("GuiPrompt")
		return nil
	})
	gui.DeleteKeybinding("", gocui.KeyEnter, gocui.ModNone)
	view.Rewind()
	gui_prompt_input <- view.Buffer()
	return nil
}

func GuiClose(gui *gocui.Gui, view *gocui.View) error {
	singleton.Closing <- true
	gui.DeleteKeybinding("", gocui.KeyCtrlC, gocui.ModNone)
	return gocui.ErrQuit
}
