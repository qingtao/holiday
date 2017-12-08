package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"holiday"
)

type WhiteList struct {
	Lock   sync.Mutex
	IPs    map[string]net.IP
	IPNets map[string]*net.IPNet
}

type Server struct {
	*http.ServeMux
	Logger *log.Logger

	WhiteList *WhiteList
	exit      chan struct{}
}

func (wl *WhiteList) Update(action, s string) error {
	wl.Lock.Lock()
	defer wl.Lock.Unlock()
	ips, ipnets := make(map[string]net.IP), make(map[string]*net.IPNet)
	for _, line := range strings.Fields(s) {
		if strings.HasPrefix(line, "#") {
			continue
		}
		_, ipNet, err := net.ParseCIDR(line)
		if err != nil {
			allowIP := net.ParseIP(line)
			if allowIP == nil {
				return fmt.Errorf("contain not a valid ip address: %s\n", err)
			}
			ips[line] = allowIP
			continue
		}
		ipnets[line] = ipNet
	}

	switch action {
	case "ADD", "UPDATE":
		for k, v := range ips {
			wl.IPs[k] = v
		}
		for k, v := range ipnets {
			wl.IPNets[k] = v
		}
	case "DEL":
		for k, _ := range ips {
			delete(wl.IPs, k)
		}
		for k, _ := range ipnets {
			delete(wl.IPNets, k)
		}
	}
	return nil
}

//读取白名单: 每一行一个IP或者网段
func NewServer(whitelist string, l *log.Logger) (*Server, error) {
	mux := http.NewServeMux()
	b, err := ioutil.ReadFile(whitelist)
	if err != nil {
		return nil, err
	}

	var srv = new(Server)
	srv.ServeMux = mux

	wl := &WhiteList{
		IPs:    make(map[string]net.IP),
		IPNets: make(map[string]*net.IPNet),
	}
	if err = wl.Update("ADD", string(b)); err != nil {
		return nil, err
	}
	srv.WhiteList = wl
	srv.Logger = l
	srv.exit = make(chan struct{})
	return srv, nil
}

//检查ip权限
func (srv *Server) Verify(ip net.IP) bool {
	for _, ipnet := range srv.WhiteList.IPNets {
		if ipnet.Contains(ip) {
			return true
		}
	}
	//for j := 0; j < len(srv.wl.IPs); j++ {
	for _, ip1 := range srv.WhiteList.IPs {
		if ip1.Equal(ip) {
			return true
		}
	}
	return false
}

//实现ServeHTTP: 并检查IP地址是否允许访问
func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var ip string
	if xforwarfor := r.Header.Get("X-Forward-For"); xforwarfor != "" {
		ip = xforwarfor
		if srv.Verify(net.ParseIP(xforwarfor)) {
			srv.ServeMux.ServeHTTP(w, r)
		}
		return
	}
	userip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		srv.Logger.Printf("client ip: %q is not IP:port", r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	ip = userip
	if srv.Verify(net.ParseIP(ip)) {
		srv.ServeMux.ServeHTTP(w, r)
		return
	}
	srv.Logger.Printf("client %s connect not allowed\n", ip)
	http.Error(w, fmt.Sprintf("ip no allowed: %s", ip), http.StatusForbidden)
}

func ServeHolidays(w http.ResponseWriter, r *http.Request, h map[int]*holiday.HolidaysOfYear) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	year := r.FormValue("year")
	if year == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "year is empty")
		return
	}
	year = strings.ToLower(year)
	y, err := strconv.Atoi(year)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "year is not a integer")
		return
	}

	hd, ok := h[y]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%d holidays is not exists", y)
		return
	}
	var i interface{} = hd

	month := r.FormValue("month")
	if month != "" {
		m, err := strconv.Atoi(month)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "month is not a integer")
			return
		}
		hm, ok := hd.Month[m]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%d%d holidays is not exists", y, m)
			return
		}
		i = hm
		day := r.FormValue("day")
		if day != "" {
			d, err := strconv.Atoi(day)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "day is not a integer")
				return
			}
			hh, ok := hm[d]
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "%d%d%d day is not a holiday", y, m, d)
				return
			}
			i = hh
		}
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%d%d holidays is not exists", y, m)
			return
		}

	}
	b, err := json.Marshal(i)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "%s", err)
		return
	}
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	fmt.Fprintf(w, "%s", b)
}

func main() {
	file := "whitelist"
	l := log.New(os.Stdout, "", 1|2)
	s, err := NewServer(file, l)
	if err != nil {
		log.Fatalln(err)
	}
	hs := map[int]*holiday.HolidaysOfYear{
		2018: holiday.NewHolidaysOfYear(2018, 0, 6),
	}

	fs, err := filepath.Glob("2018/2018*.json")
	if err != nil {
		log.Fatalln(err)
	}
	for i := 0; i < len(fs); i++ {
		b, err := ioutil.ReadFile(fs[i])
		if err != nil {
			log.Fatalln(err)
		}
		var chd holiday.ChineseHoliday
		if err := json.Unmarshal(b, &chd); err != nil {
			log.Fatalln(err)
		}
		hs[2018].Update(&chd)
	}

	s.HandleFunc("/h", func(w http.ResponseWriter, r *http.Request) {
		ServeHolidays(w, r, hs)
	})
	addr := ":10082"
	http.ListenAndServe(addr, s)
}
