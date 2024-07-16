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

var tmpl = template.Must(template.New("nouns").Parse(`package dictionary

var Words = []string{ 
{{range $word := .}}	"{{$word}}",
{{end}}
}
`))

func main() {

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic(errors.New("unable to get the current filename").Error())
	}

	f, err := os.Open(path.Join(filepath.Dir(filename), "nouns.json"))
	if err != nil {
		panic("failed to open nouns: " + err.Error())
	}
	defer f.Close()

	var words []string
	if err := json.NewDecoder(f).Decode(&words); err != nil {
		panic(err.Error())
	}

	if err := tmpl.Execute(os.Stdout, words); err != nil {
		panic("failed to execute template: " + err.Error())
	}

}
