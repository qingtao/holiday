//Package holiday 通过初始化周末休息日，
//更新中国法定节假日生成一个可查询的放假安排时间表
package holiday

import (
	"fmt"
	"time"
)

const (
	legalHoliday = 2
	holiday      = 1
)

//HolidaysOfMonth 一个月的节假日
type HolidaysOfMonth map[int]int

//HolidaysOfYear 一年的节假日，Year指定年
type HolidaysOfYear struct {
	Year  int                     `json:"year"`
	Month map[int]HolidaysOfMonth `json:"month"`
}

//NewHolidaysOfYear 生成新的常规双休(h1=0|h2=6)或者单休的节假日统计，h1=h2时按单休生成
func NewHolidaysOfYear(y, h1, h2 int) *HolidaysOfYear {
	loc := time.Now().Location()
	var yday = make(map[int]HolidaysOfMonth)

	for i := 0; i < 12; i++ {
		mday := make(map[int]int)
		yday[i+1] = mday
	}

	date := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	nextYear := date.AddDate(1, 0, 0)
	daysOfYear := nextYear.Sub(date) / time.Minute / (60 * 24)
	for i := 0; i < int(daysOfYear); i++ {
		d := date.AddDate(0, 0, i+1)
		month := yday[int(d.Month())]
		if wday := int(d.Weekday()); wday == h1 || wday == h2 {
			month[d.Day()] = holiday
		}
	}
	return &HolidaysOfYear{y, yday}
}

//Holiday 节日放假安排
type Holiday struct {
	//初始日期的月份
	Month int `json:"month"`
	//放假开始的日期，不含月份
	Start int `json:"start"`
	//放假天数
	Len int `json:"length"`
}

//WorkDay 用于更新调休上班
type WorkDay struct {
	Month int `json:"month"`
	Day   int `json:"day"`
}

//ChineseHoliday 中国节日放假安排
type ChineseHoliday struct {
	//年
	Year int `json:"year"`
	//节日名称
	Name string `json:"name"`
	//放假安排
	Holidays []Holiday `json:"holidays"`
	//法定假日
	LegalHolidays []Holiday `json:"legalholidays"`
	//此节日调休上班时间，调休大多1-3天，按照每天一个WorkDay计算
	WorkDays []WorkDay `json:"workdays"`
}

//Update 使用ch更新hy *HolidaysOfYear
func (hy *HolidaysOfYear) Update(ch *ChineseHoliday) error {
	if hy.Year != ch.Year {
		return fmt.Errorf("the year is not matched")
	}
	for _, v := range ch.Holidays {
		start := time.Date(hy.Year, time.Month(v.Month), v.Start, 0, 0, 0, 0, time.Now().Location())
		for i := 0; i < v.Len; i++ {
			d := start.AddDate(0, 0, i)
			month, ok := hy.Month[int(d.Month())]
			if ok {
				month[d.Day()] = holiday
			}
		}
	}
	for _, v := range ch.LegalHolidays {
		start := time.Date(hy.Year, time.Month(v.Month), v.Start, 0, 0, 0, 0, time.Now().Location())
		for i := 0; i < v.Len; i++ {
			d := start.AddDate(0, 0, i)
			month, ok := hy.Month[int(d.Month())]
			if ok {
				month[d.Day()] = legalHoliday
			}
		}
	}

	for _, v := range ch.WorkDays {
		month, ok := hy.Month[v.Month]
		if ok {
			delete(month, v.Day)
		}
	}
	return nil
}
