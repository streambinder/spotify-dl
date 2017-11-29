package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jroimartin/gocui"
)

var (
	gui            *gocui.Gui
	gui_max_weight int
	gui_max_height int
)

func main() {
	go GuiBuild()

	// do things
	time.Sleep(10 * time.Second)
}

func GuiBuild() {
	gui, gui_err := gocui.NewGui(gocui.OutputNormal)
	if gui_err != nil {
		log.Panicln(gui_err)
	}
	defer gui.Close()

	gui_max_weight, gui_max_height = gui.Size()

	gui.SetManagerFunc(GuiSTDLayout)

	if gui_err := gui.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, GuiClose); gui_err != nil {
		log.Panicln(gui_err)
	}

	if gui_err := gui.MainLoop(); gui_err != nil {
		if gui_err == gocui.ErrQuit {
			time.Sleep(1 * time.Second)
			gui.Close()
			os.Exit(0)
		} else {
			log.Panicln(gui_err)
		}
	}
}

func GuiSTDLayout(gui *gocui.Gui) error {
	if view_lefttop, err := gui.SetView("LeftTop", 0, 0, gui_max_weight/3, gui_max_height/2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		fmt.Fprintln(view_lefttop, "Hello world!")
	}
	if view_leftbottom, err := gui.SetView("LeftBottom", 0, gui_max_height/2+1, gui_max_weight/3, gui_max_height-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		fmt.Fprintln(view_leftbottom, "Hello world!")
	}
	if view_right, err := gui.SetView("Right", gui_max_weight/3+1, 0, gui_max_weight-1, gui_max_height-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		fmt.Fprintln(view_right, "Hello world!")
	}
	return nil
}

func GuiClose(gui *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
