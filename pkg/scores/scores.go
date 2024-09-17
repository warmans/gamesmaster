package scores

import (
	"fmt"
	"slices"
	"strings"
)

type Score struct {
	Points  int
	Answers int
}

func NewTiered(totalAnswers int) *Tiered {
	return &Tiered{TotalAnswers: totalAnswers, Scores: make(map[string]*Score)}
}

type Tiered struct {
	TotalAnswers int
	Scores       map[string]*Score
}

func (t *Tiered) points(numCompleted float64) int {
	firstTier := float64(t.TotalAnswers) / 3
	secondTier := firstTier * 2

	points := 1
	if numCompleted >= firstTier && numCompleted < secondTier {
		points = 2
	}
	if numCompleted >= secondTier {
		points = 3
	}
	return points
}

func (t *Tiered) Add(userName string) {
	numCompleted := float64(0)
	for _, v := range t.Scores {
		numCompleted += float64(v.Answers)
	}

	if _, exists := t.Scores[userName]; !exists {
		t.Scores[userName] = &Score{Points: t.points(numCompleted), Answers: 1}
	} else {
		t.Scores[userName].Points += t.points(numCompleted)
		t.Scores[userName].Answers++
	}
}

func (t *Tiered) Render() string {
	var scoreSlice []struct {
		score    *Score
		userName string
	}
	for userName, score := range t.Scores {
		scoreSlice = append(scoreSlice, struct {
			score    *Score
			userName string
		}{score: score, userName: userName})
	}

	slices.SortFunc(scoreSlice, func(a, b struct {
		score    *Score
		userName string
	}) int {
		if a.score.Points < b.score.Points {
			return 1
		}
		return -1
	})

	sb := &strings.Builder{}
	for k, v := range scoreSlice {
		fmt.Fprintf(sb, "%d. %s: %d (%d answered)\n", k+1, v.userName, v.score.Points, v.score.Answers)
	}
	return sb.String()
}
