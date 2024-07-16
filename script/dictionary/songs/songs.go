package main

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"
)

type SongMeta struct {
	Terms      []string `json:"Terms"`
	EpisodeIDs []string `json:"EpisodeIDs"`
	Track      struct {
		Artists []struct {
			Name string `json:"Name"`
			URI  string `json:"URI"`
		} `json:"Artists"`
		AlbumName     string `json:"AlbumName"`
		AlbumURI      string `json:"AlbumURI"`
		AlbumImageUrl string `json:"AlbumImageUrl"`
		Name          string `json:"Name"`
		TrackURI      string `json:"TrackURI"`
	} `json:"Track"`
}

var songsTmpl = template.Must(template.New("nouns").Parse(`package dictionary

var Songs = []string{ 
{{range $word := .Songs}}	"{{$word}}",
{{end}}
}

var Artists = []string{ 
{{range $word := .Artists}}	"{{$word}}",
{{end}}
}
`))

func main() {

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic(errors.New("unable to get the current filename").Error())
	}

	f, err := os.Open(path.Join(filepath.Dir(filename), "songs.json"))
	if err != nil {
		panic("failed to open nouns: " + err.Error())
	}
	defer f.Close()

	var songMeta struct {
		Songs map[string]SongMeta
	}
	if err := json.NewDecoder(f).Decode(&songMeta); err != nil {
		panic(err.Error())
	}

	songs := []string{}
	artists := []string{}
	for _, v := range songMeta.Songs {
		if !slices.Contains(songs, v.Track.Name) {
			songs = append(songs, strings.Replace(v.Track.Name, `"`, `'`, -1))
		}
		if !slices.Contains(artists, v.Track.Artists[0].Name) {
			artists = append(artists, strings.Replace(v.Track.Artists[0].Name, `"`, `'`, -1))
		}
	}

	if err := songsTmpl.Execute(
		os.Stdout,
		struct {
			Songs   []string
			Artists []string
		}{
			Songs:   songs,
			Artists: artists,
		}); err != nil {
		panic("failed to execute template: " + err.Error())
	}
}
