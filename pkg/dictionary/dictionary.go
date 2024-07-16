//go:generate sh -c "go run ../../script/dictionary/generate.go > nouns.gen.go"
package dictionary

import "math/rand"

func RandomNoun() string {
	return Words[rand.Intn(len(Words))-1]
}
