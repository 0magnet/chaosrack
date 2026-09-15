package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/0magnet/chaosrack/internal/cdp"
)

var (
	port   = flag.Int("port", 9222, "CDP port")
	target = flag.String("target", "127.0.0.1:8300", "tab URL substring")
	mode   = flag.String("mode", "", "mode")
	js     = flag.String("js", "", "expression")
	shot   = flag.String("shot", "", "screenshot")
	reload = flag.Bool("reload", false, "reload first")
	wait   = flag.Duration("wait", 0, "settle")
)

func main() {
	flag.Parse()
	c, err := cdp.Dial(*port, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	if *reload {
		c.Reload(10 * time.Second)
	}
	if *mode != "" {
		c.Eval(`(()=>{const s=document.getElementById('mode-select');s.value=` + "`" + *mode + "`" + `;s.dispatchEvent(new Event('change'));return s.value})()`)
		time.Sleep(*wait + 600*time.Millisecond)
	}
	if *js != "" {
		fmt.Printf("%v\n", c.Eval(*js))
	}
	if *shot != "" {
		img, _ := c.Screenshot()
		f, _ := os.Create(*shot)
		defer f.Close()
		_ = png.Encode(f, img)
		fmt.Println("wrote", *shot)
	}
}
