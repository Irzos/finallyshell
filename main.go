package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	mygo.App.WhenReady(func() {
		var win *mygo.Window
		app := newApp(func(fn func()) { win.Update(fn) })
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "GoSSH",
			Width:     1280,
			Height:    820,
			MinWidth:  900,
			MinHeight: 560,
			Content:   ui.View(app.view),
		})
		win.OnClosed(func() { app.closeAll() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
