package gui

import (
	"fmt"
	"log"
	"strings"

	"github.com/jroimartin/gocui"
)

const (
	PromptNotDismissable = iota
	PromptDismissable
	PromptDismissableWithExit
	_

	PanelLeftTop = iota
	PanelLeftBottom
	PanelRight
	_
	OrientationLeft = iota
	OrientationCenter
	OrientationRight
)

var (
	Panels = map[int]string{
		PanelLeftTop:    "GuiPanelLeftTop",
		PanelLeftBottom: "GuiPanelLeftBottom",
		PanelRight:      "GuiPanelRight",
	}

	gui_ready          chan *gocui.Gui
	gui_prompt_dismiss chan bool
)

type Gui struct {
	*gocui.Gui
	Width  int
	Height int
}

func Build() *Gui {
	var gui *gocui.Gui
	gui_ready = make(chan *gocui.Gui)
	go Run()
	gui = <-gui_ready
	gui_width, gui_height := gui.Size()

	return &Gui{
		gui,
		gui_width,
		gui_height,
	}
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
		if err == gocui.ErrQuit {
			gui.Close()
		} else {
			log.Panicln(err)
		}
	}
}

func (gui *Gui) ClearAppend(message string, panel int, orientation ...int) error {
	view, err := gui.View(Panels[panel])
	if err != nil {
		return err
	} else {
		view.Clear()
		gui.Update(func(gui *gocui.Gui) error {
			if len(orientation) > 0 {
				message = MessageOrientate(message, view, orientation[0])
			}
			fmt.Fprintln(view, message)
			return nil
		})
	}
	return nil
}

func (gui *Gui) Append(message string, panel int, orientation ...int) error {
	view, err := gui.View(Panels[panel])
	if err != nil {
		return err
	} else {
		gui.Update(func(gui *gocui.Gui) error {
			if len(orientation) > 0 {
				message = MessageOrientate(message, view, orientation[0])
			}
			fmt.Fprintln(view, message)
			return nil
		})
	}
	return nil
}

func (gui *Gui) Prompt(message string, dismiss int) error {
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
			fmt.Fprintln(view, message)
			if dismiss == PromptDismissable {
				gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPrompt)
			} else if dismiss == PromptDismissableWithExit {
				gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, GuiDismissPromptAndClose)
			}
		}
		return nil
	})
	<-gui_prompt_dismiss
	return nil
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
	line_length, _ := view.Size()
	if len(message) >= line_length {
		message = message[:line_length]
	} else {
		line_spacing := (line_length - len(message)) / 2
		if orientation == OrientationLeft {
			message = message + strings.Repeat(" ", line_spacing*2)
		} else if orientation == OrientationCenter {
			message = strings.Repeat(" ", line_spacing) +
				message + strings.Repeat(" ", line_spacing)
		} else if orientation == OrientationRight {
			message = strings.Repeat(" ", line_spacing*2) + message
		}
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

func GuiClose(gui *gocui.Gui, view *gocui.View) error {
	return gocui.ErrQuit
}
