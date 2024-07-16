//go:generate sh -c "go run ../../script/dictionary/wordlist/generate.go nouns.json Nouns > nouns.gen.go"
//go:generate sh -c "go run ../../script/dictionary/wordlist/generate.go objects.json Objects > objects.gen.go"
//go:generate sh -c "go run ../../script/dictionary/songs/songs.go > songs.gen.go"
package dictionary

import "math/rand"

func RandomNoun() string {
	return Nouns[rand.Intn(len(Nouns))-1]
}

func RandomObject() string {
	return Objects[rand.Intn(len(Objects))-1]
}

func RandomSong() string {
	return Songs[rand.Intn(len(Songs))-1]
}

func RandomArtist() string {
	return Artists[rand.Intn(len(Artists))-1]
}
