// Package main 根据双休日或者单休日生成节假日，然后从国家放假安排的每个节日的json格式修正数据，从而导出最终的时间表
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

var (
	dir       = flag.String("dir", "./data", "放假数据目录")
	whitelist = flag.String("whitelist", "./whitelist.cnf", "白名单文件")
	h1        = flag.Int("weekend1", 0, "每周休息日的第一天,默认0(周日)")
	h2        = flag.Int("weekend2", 6, "每周休息日的第二天,默认6(周六)")
)

func main() {
	flag.Parse()
	l := log.New(os.Stdout, "", 1|2)
	s, err := NewServer(*whitelist, l)
	if err != nil {
		log.Fatalln(err)
	}
	// fmt.Printf("%+v\n", s.WhiteList)
	hs, err := load(*dir, *h1, *h2)
	if err != nil {
		log.Fatalln(err)
	}

	s.HandleFunc("/holidays", func(w http.ResponseWriter, r *http.Request) {
		if !s.VerifyClient(w, r) {
			return
		}
		ServeHolidays(w, r, hs)
	})
	s.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ServeMain(w, r, "hello!")
	})
	addr := "127.0.0.1:8080"
	http.ListenAndServe(addr, s)
}
