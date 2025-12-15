package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/ghostlawless/xdl/internal/app"
)

type pack struct {
	Ref string
	Buf []byte
}

func spin() pack {
	var x [32]byte
	if _, e := rand.Read(x[:]); e != nil {
		return pack{
			Ref: "run",
			Buf: []byte("fallback-run-seed"),
		}
	}

	r := hex.EncodeToString(x[:3])
	return pack{
		Ref: r,
		Buf: x[:],
	}

}

func main() {
	p := spin()
	id := p.Ref
	b := p.Buf

	defer func() {
		if v := recover(); v != nil {
			fmt.Fprintln(os.Stderr, "fatal:", v)
			if s := debug.Stack(); len(s) > 0 {
				_, _ = os.Stderr.Write(s)
			}
			os.Exit(1)
		}
	}()

	if err := app.RunWithArgsAndID(os.Args[1:], id, b); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}
