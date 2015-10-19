package main

import (
	"flag"
	"os"
	"runtime"
	"strings"
)

var (
	bsHost       string
	bsPort       int
	pollInterval int
)

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.StringVar(&bsHost, "h", "127.0.0.1", "beanstalkd host")
	flag.IntVar(&bsPort, "p", 11300, "beanstalkd port")
	flag.IntVar(&pollInterval, "i", 1, "refresh interval in seconds and must be greater than 1 second")
	flag.Parse()
	if strings.TrimSpace(bsHost) == "" {
		flag.PrintDefaults()
		os.Exit(-1)
	}

	if pollInterval < 1 {
		pollInterval = 1
	}

	(&mainFrame{}).show(bsHost, bsPort, pollInterval)
}
