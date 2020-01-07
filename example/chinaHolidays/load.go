package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/qingtao/holiday"
)

// load 加载数据
func load(dir string, h1, h2 int) (hs map[int]*holiday.HolidaysOfYear, err error) {
	hs = make(map[int]*holiday.HolidaysOfYear)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		var chd holiday.ChineseHoliday
		if err := json.Unmarshal(b, &chd); err != nil {
			return err
		}
		if _, ok := hs[chd.Year]; !ok {
			hs[chd.Year] = holiday.NewHolidaysOfYear(chd.Year, h1, h2)
		}
		hs[chd.Year].Update(&chd)
		return nil
	})
	return
}
