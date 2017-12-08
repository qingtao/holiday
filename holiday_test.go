package holiday

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestUpdateHoliday(t *testing.T) {
	eh := Holiday{2, 15, 7}
	el := Holiday{2, 16, 3}
	ew := []WorkDay{
		{2, 11},
		{2, 24},
	}
	ih := &ChineseHoliday{
		Year:          2018,
		Name:          "春节",
		Holidays:      []Holiday{eh},
		LegalHolidays: []Holiday{el},
		WorkDays:      ew,
	}

	b, err := json.MarshalIndent(ih, "", "  ")
	if err != nil {
		t.Fatalf("%s\n", err)
		return
	}
	fmt.Printf("%s\n", b)

	hd := NewHolidaysOfYear(2018, 0, 6)
	if err := hd.Update(ih); err != nil {
		t.Fatalf("%s\n", err)
		return
	}
	b, err = json.MarshalIndent(hd.Month[2], "", "  ")
	if err != nil {
		t.Fatalf("%s\n", err)
	}
	fmt.Printf("%s\n", b)
}
