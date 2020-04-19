package scheduler

import "testing"

func Test_checkRange(t *testing.T) {
	type args struct {
		v int
		r Range
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "inside range",
			args: args{
				v: 17,
				r: Range{14, 19},
			},
			want: true,
		},
		{
			name: "inside range left boundary",
			args: args{
				v: 14,
				r: Range{14, 19},
			},
			want: true,
		},
		{
			name: "outside range left",
			args: args{
				v: 13,
				r: Range{14, 19},
			},
			want: false,
		},
		{
			name: "outside range right",
			args: args{
				v: 20,
				r: Range{14, 19},
			},
			want: false,
		},
		{
			name: "outside range right boundary",
			args: args{
				v: 19,
				r: Range{14, 19},
			},
			want: false,
		},
		{
			name: "inside inverted range right",
			args: args{
				v: 20,
				r: Range{19, 10},
			},
			want: true,
		},
		{
			name: "inside inverted range left",
			args: args{
				v: 9,
				r: Range{19, 10},
			},
			want: true,
		},
		{
			name: "outside inverted range",
			args: args{
				v: 11,
				r: Range{19, 10},
			},
			want: false,
		},
		{
			name: "outside inverted range left boundary",
			args: args{
				v: 10,
				r: Range{19, 10},
			},
			want: false,
		},
		{
			name: "inside inverted range right boundary",
			args: args{
				v: 19,
				r: Range{19, 10},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkRange(tt.args.v, tt.args.r); got != tt.want {
				t.Errorf("checkRange() = %v, want %v", got, tt.want)
			}
		})
	}
}
