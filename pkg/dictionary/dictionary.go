//go:generate sh -c "go run ../../script/dictionary/nouns.go > nouns.gen.go"
//go:generate sh -c "go run ../../script/dictionary/songs.go > songs.gen.go"
package dictionary

import "math/rand"

func RandomNoun() string {
	return Words[rand.Intn(len(Words))-1]
}

func RandomSong() string {
	return Songs[rand.Intn(len(Songs))-1]
}

func RandomArtist() string {
	return Artists[rand.Intn(len(Artists))-1]
}
