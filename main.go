package main

import (
	"flag"
	"os"
	"strings"
)

var (
	bsHost string
	bsPort int
)

func main() {

	flag.StringVar(&bsHost, "h", "127.0.0.1", "beanstalkd host")
	flag.IntVar(&bsPort, "p", 11300, "beanstalkd port")
	flag.Parse()
	if strings.TrimSpace(bsHost) == "" {
		flag.PrintDefaults()
		os.Exit(-1)
	}

	(&mainFrame{}).show(bsHost, bsPort)
}
