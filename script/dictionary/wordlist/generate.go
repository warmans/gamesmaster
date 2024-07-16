package main

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"text/template"
)

var nounsTmpl = template.Must(template.New("words").Parse(`package dictionary

var {{.Name}} = []string{ 
{{range $word := .Words}}	"{{$word}}",
{{end}}
}
`))

func main() {

	if len(os.Args) < 3 {
		panic("not enough arguments e.g. generate.go words.json Words")
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic(errors.New("unable to get the current filename").Error())
	}

	f, err := os.Open(path.Join(filepath.Dir(filename), os.Args[1]))
	if err != nil {
		panic("failed to open nouns: " + err.Error())
	}
	defer f.Close()

	var words []string
	if err := json.NewDecoder(f).Decode(&words); err != nil {
		panic(err.Error())
	}

	if err := nounsTmpl.Execute(
		os.Stdout,
		struct {
			Name  string
			Words []string
		}{
			Name:  os.Args[2],
			Words: words,
		}); err != nil {
		panic("failed to execute template: " + err.Error())
	}

}
