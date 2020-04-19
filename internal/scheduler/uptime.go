package scheduler

import (
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Range struct {
	From int
	To   int
}

type UptimeRange struct {
	Hours    Range
	Days     Range
	Months   Range
	Weekdays Range
}

func parseRange(spec string, min, max int) (*Range, error) {
	// 'x' - full range
	if spec == "x" {
		return &Range{min, max}, nil
	}
	// split range
	r := strings.Split(spec, "-")
	if len(r) != 2 {
		return nil, errors.New("invalid range, must be of form 'from-to'")
	}
	from, err := strconv.Atoi(r[0])
	if err != nil {
		return nil, errors.Wrap(err, "the first element in the range is not valid")
	}
	if from < min || from > max {
		return nil, errors.Errorf("'from' out of allowed range: %d-%d", min, max)
	}
	to, err := strconv.Atoi(r[1])
	if err != nil {
		return nil, errors.Wrap(err, "the second element in the range is not valid")
	}
	if to < min || to > max {
		return nil, errors.Errorf("'to' out of allowed range: %d-%d", min, max)
	}
	if to == from {
		return nil, errors.New("'from' cannot be equal to 'to'")
	}

	return &Range{from, to}, nil
}

func ParseUptime(spec string) (*UptimeRange, error) {
	uptime := strings.Split(spec, "_")
	if len(uptime) != 4 {
		return nil, errors.New("uptime should contain 4 ranges: hours_weekday_days_months")
	}
	hours, err := parseRange(uptime[0], 0, 24)
	if err != nil {
		return nil, errors.Wrap(err, "invalid hours range, must be between 0-23")
	}
	weekdays, err := parseRange(uptime[1], 0, 6)
	if err != nil {
		return nil, errors.Wrap(err, "invalid weekdays range, must be between 0-6")
	}
	days, err := parseRange(uptime[2], 1, 31)
	if err != nil {
		return nil, errors.Wrap(err, "invalid days range, must be between 1-31")
	}
	months, err := parseRange(uptime[3], 1, 12)
	if err != nil {
		return nil, errors.Wrap(err, "invalid months range, must be between 1-12")
	}

	return &UptimeRange{
		Hours:    *hours,
		Days:     *days,
		Months:   *months,
		Weekdays: *weekdays}, nil
}

func checkRange(v int, r Range) bool {
	if r.From < r.To {
		if v < r.From || v >= r.To {
			return false
		}
	} else {
		if v < r.From && v >= r.To {
			return false
		}
	}
	return true
}

func (uptime *UptimeRange) IsInRange(t time.Time) bool {
	// check hours range
	if !checkRange(t.Hour(), uptime.Hours) {
		return false
	}
	// check days range
	if !checkRange(t.Day(), uptime.Days) {
		return false
	}
	// check month range
	if !checkRange(int(t.Month()), uptime.Months) {
		return false
	}
	// check weekdays range
	if !checkRange(int(t.Weekday()), uptime.Weekdays) {
		return false
	}
	// in range
	return true
}
