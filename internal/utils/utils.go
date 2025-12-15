package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	prefixNormal = "xdl>"
	prefixAlert  = "xdl!"
)

func printTo(writer *os.File, prefix string, format string, args ...any) {
	fmt.Fprintf(writer, "%s %s\n", prefix, fmt.Sprintf(format, args...))
}

func PrintInfo(format string, args ...any) {
	printTo(os.Stdout, prefixNormal, format, args...)
}

func PrintSuccess(format string, args ...any) {
	printTo(os.Stdout, prefixNormal, format, args...)
}

func PrintWarn(format string, args ...any) {
	printTo(os.Stderr, prefixAlert, format, args...)
}

func PrintError(format string, args ...any) {
	printTo(os.Stderr, prefixAlert, format, args...)
}

func PrintBanner() {
	const banner = `
           /$$$$$$$  
          | $$__  $$ 
 /$$   /$$| $$  \ $$
|  $$ /$$/| $$  | $$
 \  $$$$/ | $$  | $$
  >$$  $$ | $$  | $$
 /$$/\  $$| $$$$$$$/
|__/  \__/|_______/ 

xdl > x Downloader
`
	fmt.Fprint(os.Stdout, banner+"\n")
}

func PromptYesNoDefaultYes(question string) bool {
	fmt.Fprint(os.Stdout, question)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "" || line == "y" || line == "yes"
}
