package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
)

var version = "1.0.0"

func main() {
	log.SetFlags(log.Ltime)
	log.SetOutput(os.Stdout)

	if runHeadless() {
		return
	}

	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.SetName("FO3启动器")
	mygo.App.SetVersion(version)

	a := newApp()

	mygo.App.OnSecondInstance(func([]string, string) { a.focusWindow() })
	mygo.App.OnWillQuit(func(e *mygo.QuitEvent) { a.shutdown() })
	mygo.App.WhenReady(a.ready)

	if err := mygo.App.Run(); err != nil {
		log.Printf("启动器退出：%v", err)
		os.Exit(1)
	}
}
