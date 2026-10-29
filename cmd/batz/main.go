package main

import (
	"batz/internal/app"
	"os"
)

func main() { os.Exit(app.Execute(os.Args[1:], os.Stdout, os.Stderr)) }
