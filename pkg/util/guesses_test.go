package util

import "testing"

func TestGuessRoughlyMatchesAnswer(t *testing.T) {
	type args struct {
		guess  string
		answer string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "exact match",
			args: args{
				guess:  "the answer",
				answer: "the answer",
			},
			want: true,
		},
		{
			name: "slightly wrong suffix matches",
			args: args{
				guess:  "bill oddy",
				answer: "bill oddie",
			},
			want: true,
		},
		{
			name: "slightly wrong prefix matches",
			args: args{
				guess:  "bill oddy",
				answer: "gill oddy",
			},
			want: true,
		},
		{
			name: "completely wrong doesn't match",
			args: args{
				guess:  "peter sissons",
				answer: "bill oddie",
			},
			want: false,
		},
		{
			name: "case does not matter",
			args: args{
				guess:  "BiLL oddY",
				answer: "bill oddy",
			},
			want: true,
		},
		{
			name: "filler words not allowed in the middle",
			args: args{
				guess:  "james bond",
				answer: "james the bond",
			},
			want: false,
		},
		{
			name: "filler words allowed at the start",
			args: args{
				guess:  "james bond",
				answer: "the james bond",
			},
			want: true,
		},
		{
			name: "the lighthouse",
			args: args{
				guess:  "The lighthouse",
				answer: "the lighthouse",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GuessRoughlyMatchesAnswer(tt.args.guess, tt.args.answer); got != tt.want {
				t.Errorf("GuessRoughlyMatchesAnswer() = %v, want %v", got, tt.want)
			}
		})
	}
}
